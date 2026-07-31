package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/sender-api/sender-api/internal/auth"
	"github.com/sender-api/sender-api/internal/config"
	"github.com/sender-api/sender-api/internal/handler"
	"github.com/sender-api/sender-api/internal/mailer"
	"github.com/sender-api/sender-api/internal/queue"
	"github.com/sender-api/sender-api/internal/repository"
	"github.com/sender-api/sender-api/internal/service"
	pkgmiddleware "github.com/sender-api/sender-api/pkg/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg := config.Load()

	if cfg.SentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.SentryDSN,
			Environment:      cfg.Env,
			TracesSampleRate: 0.2,
		}); err != nil {
			logger.Warn("failed to init Sentry", "error", err)
		} else {
			defer sentry.Flush(2 * time.Second)
			logger.Info("Sentry initialized")
		}
	}

	logger.Info("starting sender-api",
		"env", cfg.Env,
		"port", cfg.Port,
	)

	if err := auth.InitJWT(cfg.SupabaseURL); err != nil {
		logger.Warn("failed to init JWT (continuing without JWT auth)", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	logger.Info("connected to database")

	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Warn("failed to connect to redis (continuing without queue)", "error", err)
	}

	awsCfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Warn("failed to load aws config", "error", err)
	}

	sesClient := sesv2.NewFromConfig(awsCfg)

	emailRepo := repository.NewEmailRepo(dbPool)
	teamRepo := repository.NewTeamRepo(dbPool)
	contactRepo := repository.NewContactRepo(dbPool)
	domainRepo := repository.NewDomainRepo(dbPool)
	apiKeyRepo := repository.NewAPIKeyRepo(dbPool)
	inboundRepo := repository.NewInboundEmailRepo(dbPool)
	webhookRepo := repository.NewWebhookRepo(dbPool)

	auth.SetVerifyAPIKeyContextFunc(func(ctx context.Context, rawKey string) (*auth.APIKeyContext, error) {
		verification, err := apiKeyRepo.VerifyAPIKey(ctx, rawKey)
		if err != nil {
			return nil, err
		}
		if apiKeyID, err := uuid.Parse(verification.APIKeyID); err == nil {
			if err := apiKeyRepo.UpdateLastUsed(ctx, apiKeyID); err != nil {
				logger.Warn("failed to update api key last used", "api_key_id", apiKeyID, "error", err)
			}
		}
		return &auth.APIKeyContext{
			TeamID:      verification.TeamID,
			APIKeyID:    verification.APIKeyID,
			Permissions: verification.Permissions,
			Plan:        verification.Plan,
		}, nil
	})

	auth.SetUserTeamResolver(func(ctx context.Context, userID, requestedTeamID string) (*auth.TeamContext, error) {
		uid, err := uuid.Parse(userID)
		if err != nil {
			return nil, fmt.Errorf("invalid user id: %w", err)
		}
		teamID, err := uuid.Parse(requestedTeamID)
		if err != nil {
			return nil, fmt.Errorf("invalid team id: %w", err)
		}
		member, err := teamRepo.GetMember(ctx, teamID, uid)
		if err != nil {
			return nil, fmt.Errorf("team membership not found: %w", err)
		}
		team, err := teamRepo.GetByID(ctx, teamID)
		if err != nil {
			return nil, fmt.Errorf("team not found: %w", err)
		}
		return &auth.TeamContext{
			TeamID: teamID.String(),
			Role:   string(member.Role),
			Plan:   string(team.Plan),
		}, nil
	})

	redisQueue := queue.NewRedisQueue(redisClient)

	sesMailer := mailer.NewSESMailer(sesClient, cfg.AWSESConfigSet, logger)

	emailService := service.NewEmailService(emailRepo, redisQueue, sesMailer, webhookRepo, logger)
	teamService := service.NewTeamService(teamRepo, logger)
	contactService := service.NewContactService(contactRepo, logger)
	domainService := service.NewDomainService(domainRepo, logger)
	inboundService := service.NewInboundService(inboundRepo, domainRepo, webhookRepo, logger)

	emailHandler := handler.NewEmailHandler(emailService)
	teamHandler := handler.NewTeamHandler(teamService)
	contactHandler := handler.NewContactHandler(contactService)
	domainHandler := handler.NewDomainHandler(domainService)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyRepo)
	webhookHandler := handler.NewWebhookHandler(webhookRepo)
	inboundHandler := handler.NewInboundHandler(inboundService)

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(pkgmiddleware.CORS(cfg.CORSOrigins))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.AuthMiddleware)
		r.Use(pkgmiddleware.RateLimit(redisClient))

		r.Mount("/emails", emailHandler.Routes())
		r.Mount("/teams", teamHandler.Routes())
		r.Mount("/contacts", contactHandler.Routes())
		r.Mount("/domains", domainHandler.Routes())
		r.Mount("/api-keys", apiKeyHandler.Routes())
		r.Mount("/webhooks", webhookHandler.Routes())
		r.Mount("/inbound", inboundHandler.Routes())
	})

	r.With(
		pkgmiddleware.InboundToken(cfg.InboundWebhookToken),
		pkgmiddleware.RateLimit(redisClient),
	).Post(
		"/api/v1/inbound/ses",
		inboundHandler.HandleSESPayload,
	)

	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}

	logger.Info("server exited")
}
