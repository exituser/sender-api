package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/sender-api/sender-api/internal/service"
	"github.com/sender-api/sender-api/pkg/metrics"
)

type ProviderEventWorker struct {
	service      *service.EmailService
	logger       *slog.Logger
	pollInterval time.Duration
	metrics      *metrics.WorkerMetrics
	health       *HealthState
}

func NewProviderEventWorker(emailService *service.EmailService, logger *slog.Logger, pollInterval time.Duration) *ProviderEventWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &ProviderEventWorker{
		service: emailService, logger: logger, pollInterval: pollInterval,
		metrics: metrics.NewWorkerMetrics("provider_events"), health: NewHealthState(),
	}
}

func (w *ProviderEventWorker) Health() *HealthState { return w.health }

func (w *ProviderEventWorker) Start(ctx context.Context) {
	defer w.health.SetReady(false)
	w.logger.Info("provider event worker started")
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("provider event worker stopped")
			return
		default:
		}
		w.health.Heartbeat()
		w.metrics.Start()
		processed, err := w.service.ProcessNextProviderEvent(ctx)
		if err != nil {
			w.health.SetReady(false)
			w.metrics.Fail()
			w.logger.Error("failed to process provider event inbox", "error", err)
			sleepContext(ctx, w.pollInterval)
			continue
		}
		w.health.SetReady(true)
		w.metrics.Complete()
		if !processed {
			sleepContext(ctx, w.pollInterval)
		}
	}
}
