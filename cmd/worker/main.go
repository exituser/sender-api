package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/sender-api/sender-api/internal/config"
	"github.com/sender-api/sender-api/internal/mailer"
	"github.com/sender-api/sender-api/internal/queue"
	"github.com/sender-api/sender-api/internal/repository"
	"github.com/sender-api/sender-api/internal/service"
	"github.com/sender-api/sender-api/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger.Info("starting sender-api worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startupCtx, startupCancel := context.WithTimeout(ctx, 15*time.Second)
	defer startupCancel()

	dbConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to parse database config", "error", err)
		os.Exit(1)
	}
	dbConfig.MaxConns = int32(cfg.DBMaxConns)
	dbConfig.MinConns = int32(cfg.DBMinConns)
	dbPool, err := pgxpool.NewWithConfig(startupCtx, dbConfig)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()
	if err := dbPool.Ping(startupCtx); err != nil {
		logger.Error("failed to ping database", "error", err)
		os.Exit(1)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisURL,
		PoolSize: cfg.RedisPoolSize,
	})
	defer func() { _ = redisClient.Close() }()
	if err := redisClient.Ping(startupCtx).Err(); err != nil {
		logger.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}

	awsCfg, err := awscfg.LoadDefaultConfig(startupCtx,
		awscfg.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("failed to load aws config", "error", err)
		os.Exit(1)
	}

	sesClient := sesv2.NewFromConfig(awsCfg)

	emailRepo := repository.NewEmailRepo(dbPool)
	domainRepo := repository.NewDomainRepo(dbPool)
	inboundRepo := repository.NewInboundEmailRepo(dbPool)
	webhookRepo := repository.NewWebhookRepo(dbPool)
	webhookDeliveryRepo := repository.NewWebhookDeliveryRepo(dbPool)
	redisQueue := queue.NewRedisQueue(redisClient)
	sesMailer := mailer.NewSESMailer(sesClient, cfg.AWSESConfigSet, logger)

	emailService := service.NewEmailService(emailRepo, domainRepo, redisQueue, sesMailer, webhookRepo, webhookDeliveryRepo, logger)
	inboundService := service.NewInboundService(inboundRepo, domainRepo, webhookRepo, webhookDeliveryRepo, logger)

	emailWorker := worker.NewEmailWorker(emailService, redisQueue, logger, cfg.WorkerPollInterval)
	webhookWorker := worker.NewWebhookWorker(webhookDeliveryRepo, logger, cfg.WorkerPollInterval, cfg.IsProduction())

	var workers sync.WaitGroup
	startWorker := func(run func(context.Context)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			run(ctx)
		}()
	}
	startWorker(emailWorker.Start)
	startWorker(webhookWorker.Start)
	if cfg.InboundSQSQueueURL != "" {
		inboundWorker := worker.NewInboundWorker(
			s3.NewFromConfig(awsCfg),
			sqs.NewFromConfig(awsCfg),
			cfg.InboundSQSQueueURL,
			cfg.InboundS3Bucket,
			cfg.AWSRegion,
			cfg.InboundSNSTopicArn,
			inboundService,
			logger,
		)
		startWorker(inboundWorker.Start)
	}

	logger.Info("worker is running")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down worker...")
	cancel()
	workers.Wait()
	logger.Info("worker exited")
}
