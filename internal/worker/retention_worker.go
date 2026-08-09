package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/sender-api/sender-api/internal/service"
)

type RetentionWorker struct {
	service           *service.RetentionService
	emailAge          time.Duration
	inboundAge        time.Duration
	logger            *slog.Logger
	health            *HealthState
	runInterval       time.Duration
	heartbeatInterval time.Duration
}

const (
	retentionRunInterval       = 24 * time.Hour
	retentionHeartbeatInterval = 30 * time.Second
)

func NewRetentionWorker(retentionService *service.RetentionService, emailAge, inboundAge time.Duration, logger *slog.Logger) *RetentionWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &RetentionWorker{
		service:           retentionService,
		emailAge:          emailAge,
		inboundAge:        inboundAge,
		logger:            logger,
		health:            NewHealthState(),
		runInterval:       retentionRunInterval,
		heartbeatInterval: retentionHeartbeatInterval,
	}
}

func (w *RetentionWorker) Health() *HealthState { return w.health }

func (w *RetentionWorker) Start(ctx context.Context) {
	if w.service == nil || (w.emailAge <= 0 && w.inboundAge <= 0) {
		w.health.SetReady(true)
		w.logger.Info("retention worker disabled")
		return
	}
	defer w.health.SetReady(false)
	w.health.SetReady(w.run(ctx))
	runTicker := time.NewTicker(w.runInterval)
	defer runTicker.Stop()
	heartbeatTicker := time.NewTicker(w.heartbeatInterval)
	defer heartbeatTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-runTicker.C:
			w.health.SetReady(w.run(ctx))
		case <-heartbeatTicker.C:
			w.health.Heartbeat()
		}
	}
}

func (w *RetentionWorker) run(ctx context.Context) bool {
	if err := w.service.Purge(ctx, w.emailAge, w.inboundAge); err != nil {
		w.logger.Error("retention purge failed", "error", err)
		return false
	}
	w.logger.Info("retention purge completed", "email_age", w.emailAge, "inbound_age", w.inboundAge)
	return true
}
