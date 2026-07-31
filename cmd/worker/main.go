package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
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

	logger.Info("starting sender-api worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})
	defer redisClient.Close()

	awsCfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("failed to load aws config", "error", err)
		os.Exit(1)
	}

	sesClient := sesv2.NewFromConfig(awsCfg)

	emailRepo := repository.NewEmailRepo(dbPool)
	webhookRepo := repository.NewWebhookRepo(dbPool)
	redisQueue := queue.NewRedisQueue(redisClient)
	sesMailer := mailer.NewSESMailer(sesClient, cfg.AWSESConfigSet, logger)

	emailService := service.NewEmailService(emailRepo, redisQueue, sesMailer, webhookRepo, logger)

	emailWorker := worker.NewEmailWorker(emailService, redisQueue, logger)

	go emailWorker.Start(ctx)

	logger.Info("worker is running")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down worker...")
	cancel()
	logger.Info("worker exited")
}
