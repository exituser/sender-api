package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
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
	"github.com/sender-api/sender-api/pkg/metrics"
	pkgmiddleware "github.com/sender-api/sender-api/pkg/middleware"
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
	if err := repository.CheckSchemaVersion(startupCtx, dbPool); err != nil {
		logger.Error("database schema is not ready", "error", err)
		os.Exit(1)
	}

	redisOptions, err := config.ParseRedisOptions(cfg.RedisURL)
	if err != nil {
		logger.Error("invalid redis config", "error", err)
		os.Exit(1)
	}
	redisOptions.PoolSize = cfg.RedisPoolSize
	redisClient := redis.NewClient(redisOptions)
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
	s3Client := s3.NewFromConfig(awsCfg)

	emailRepo := repository.NewEmailRepo(dbPool)
	domainRepo := repository.NewDomainRepo(dbPool)
	inboundRepo := repository.NewInboundEmailRepo(dbPool)
	webhookRepo := repository.NewWebhookRepo(dbPool)
	webhookDeliveryRepo := repository.NewWebhookDeliveryRepo(dbPool)
	suppressionRepo := repository.NewSuppressionRepo(dbPool)
	pipelineRepo := repository.NewDeliveryPipelineRepo(dbPool)
	redisQueue := queue.NewRedisQueue(redisClient)
	sesMailer := mailer.NewSESMailer(sesClient, cfg.AWSESConfigSet, logger)

	emailService := service.NewEmailService(emailRepo, domainRepo, redisQueue, sesMailer, webhookRepo, webhookDeliveryRepo, logger, suppressionRepo)
	emailService.SetContactRepository(repository.NewContactRepo(dbPool))
	emailService.SetPlanResolver(repository.NewTeamRepo(dbPool), cfg.PlanFreeDailyLimit, cfg.PlanProDailyLimit, cfg.PlanScaleDailyLimit)
	emailService.SetDeliveryPipelineRepository(pipelineRepo)
	inboundService := service.NewInboundService(inboundRepo, domainRepo, webhookRepo, webhookDeliveryRepo, logger)
	inboundService.SetDeliveryPipelineRepository(pipelineRepo)
	retentionService := service.NewRetentionService(emailRepo, inboundRepo, webhookDeliveryRepo)
	retentionService.SetInboundObjectDeleter(service.InboundObjectDeleteFunc(func(deleteCtx context.Context, key string) error {
		_, err := s3Client.DeleteObject(deleteCtx, &s3.DeleteObjectInput{
			Bucket: aws.String(cfg.InboundS3Bucket),
			Key:    aws.String(key),
		})
		return err
	}))

	emailWorker := worker.NewEmailWorker(emailService, redisQueue, logger, cfg.WorkerPollInterval)
	webhookWorker := worker.NewWebhookWorker(webhookDeliveryRepo, logger, cfg.WorkerPollInterval, cfg.IsProduction())
	webhookWorker.Configure(worker.WithWebhookPipeline(pipelineRepo))
	providerEventWorker := worker.NewProviderEventWorker(emailService, logger, cfg.WorkerPollInterval)
	retentionWorker := worker.NewRetentionWorker(retentionService, time.Duration(cfg.EmailRetentionDays)*24*time.Hour, time.Duration(cfg.InboundRetentionDays)*24*time.Hour, logger)
	healthStates := []*worker.HealthState{emailWorker.Health(), webhookWorker.Health(), providerEventWorker.Health()}
	if cfg.EmailRetentionDays > 0 || cfg.InboundRetentionDays > 0 {
		healthStates = append(healthStates, retentionWorker.Health())
	}

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
	startWorker(providerEventWorker.Start)
	startWorker(retentionWorker.Start)
	if cfg.InboundSQSQueueURL != "" {
		inboundWorker := worker.NewInboundWorker(
			s3Client,
			sqs.NewFromConfig(awsCfg),
			cfg.InboundSQSQueueURL,
			cfg.InboundS3Bucket,
			cfg.AWSRegion,
			cfg.InboundSNSTopicArn,
			int32(cfg.InboundVisibilityTimeoutSeconds),
			inboundService,
			logger,
		)
		healthStates = append(healthStates, inboundWorker.Health())
		startWorker(inboundWorker.Start)
	}

	healthHandler := worker.NewHealthHandler(healthStates...)
	healthHandler.AddChecks(
		func(checkCtx context.Context) error { return dbPool.Ping(checkCtx) },
		func(checkCtx context.Context) error { return redisClient.Ping(checkCtx).Err() },
		func(checkCtx context.Context) error { return repository.CheckSchemaVersion(checkCtx, dbPool) },
	)
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	healthMux.Handle("/readyz", healthHandler)
	metricsHandler := http.Handler(http.HandlerFunc(metrics.Handler))
	if cfg.MetricsToken != "" {
		metricsHandler = pkgmiddleware.RequireToken(cfg.MetricsToken)(metricsHandler)
	}
	healthMux.Handle("/metrics", metricsHandler)
	healthServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.WorkerHealthPort),
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("worker health server listening", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	logger.Info("worker is running")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case err := <-serverErrors:
		logger.Error("worker health server failed", "error", err)
	}

	logger.Info("shutting down worker...")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to stop worker health server", "error", err)
	}
	shutdownCancel()
	workers.Wait()
	logger.Info("worker exited")
}
