package worker

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/metrics"
	"github.com/sender-api/sender-api/pkg/webhook"
)

type WebhookWorker struct {
	repo          domain.WebhookDeliveryRepository
	logger        *slog.Logger
	pollInterval  time.Duration
	requireHTTPS  bool
	concurrency   int
	metrics       *metrics.WorkerMetrics
	health        *HealthState
	pipeline      domain.DeliveryPipelineRepository
	recoveryReady atomic.Bool
}

type WebhookWorkerOption func(*WebhookWorker)

func WithWebhookConcurrency(value int) WebhookWorkerOption {
	return func(w *WebhookWorker) {
		if value > 0 {
			w.concurrency = value
		}
	}
}
func WithWebhookMetrics(value *metrics.WorkerMetrics) WebhookWorkerOption {
	return func(w *WebhookWorker) {
		if value != nil {
			w.metrics = value
		}
	}
}
func WithWebhookHealth(value *HealthState) WebhookWorkerOption {
	return func(w *WebhookWorker) {
		if value != nil {
			w.health = value
		}
	}
}
func WithWebhookPipeline(value domain.DeliveryPipelineRepository) WebhookWorkerOption {
	return func(w *WebhookWorker) { w.pipeline = value }
}

func NewWebhookWorker(repo domain.WebhookDeliveryRepository, logger *slog.Logger, pollInterval time.Duration, requireHTTPS ...bool) *WebhookWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	production := len(requireHTTPS) > 0 && requireHTTPS[0]
	return &WebhookWorker{repo: repo, logger: logger, pollInterval: pollInterval, requireHTTPS: production, concurrency: 4, metrics: metrics.NewWorkerMetrics("webhook"), health: NewHealthState()}
}

func NewWebhookWorkerWithOptions(repo domain.WebhookDeliveryRepository, logger *slog.Logger, pollInterval time.Duration, options ...WebhookWorkerOption) *WebhookWorker {
	w := NewWebhookWorker(repo, logger, pollInterval)
	w.Configure(options...)
	return w
}

func (w *WebhookWorker) Configure(options ...WebhookWorkerOption) {
	for _, option := range options {
		option(w)
	}
}

func (w *WebhookWorker) Health() *HealthState { return w.health }

func (w *WebhookWorker) Start(ctx context.Context) {
	defer w.health.SetReady(false)
	w.logger.Info("webhook worker started")
	ready := true
	if err := w.repo.RecoverStale(ctx); err != nil {
		ready = false
		w.logger.Error("failed to recover stale webhook deliveries", "error", err)
	}
	if w.pipeline != nil {
		if err := w.pipeline.RecoverStalePipelineWork(ctx); err != nil {
			ready = false
			w.logger.Error("failed to recover stale webhook pipeline work", "error", err)
		}
	}
	w.health.SetReady(ready)
	w.recoveryReady.Store(ready)
	maintenance := time.NewTicker(30 * time.Second)
	defer maintenance.Stop()
	maintenanceDone := make(chan struct{})
	defer close(maintenanceDone)
	go func() {
		for {
			select {
			case <-maintenance.C:
				maintenanceReady := true
				if err := w.repo.RecoverStale(ctx); err != nil {
					maintenanceReady = false
					w.logger.Error("failed to recover stale webhook deliveries", "error", err)
				}
				if w.pipeline != nil {
					if err := w.pipeline.RecoverStalePipelineWork(ctx); err != nil {
						maintenanceReady = false
						w.logger.Error("failed to recover stale webhook pipeline work", "error", err)
					}
				}
				w.health.SetReady(maintenanceReady)
				w.recoveryReady.Store(maintenanceReady)
			case <-maintenanceDone:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	var workers sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); w.run(ctx) }()
	}
	workers.Wait()
}

func (w *WebhookWorker) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("webhook worker stopped")
			return
		default:
		}
		w.health.Heartbeat()
		if w.pipeline != nil {
			if _, err := w.pipeline.DispatchNextOutbox(ctx); err != nil {
				w.health.SetReady(false)
				w.logger.Error("failed to dispatch webhook outbox", "error", err)
				sleepContext(ctx, w.pollInterval)
				continue
			}
		}

		delivery, err := w.repo.ClaimDelivery(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			w.health.SetReady(w.recoveryReady.Load())
			sleepContext(ctx, w.pollInterval)
			continue
		}
		if err != nil {
			w.health.SetReady(false)
			w.logger.Error("failed to claim webhook delivery", "error", err)
			sleepContext(ctx, w.pollInterval)
			continue
		}
		w.health.SetReady(w.recoveryReady.Load())

		started := time.Now()
		w.metrics.Start()
		deliveryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = webhook.SendWebhookWithIDPolicy(deliveryCtx, delivery.ID, delivery.URL, delivery.Secret, delivery.Event, delivery.Payload, w.requireHTTPS)
		cancel()
		if err == nil {
			w.metrics.Complete()
			w.metrics.ObserveAge(time.Since(started))
			if markErr := w.repo.MarkDelivered(ctx, delivery.ID); markErr != nil {
				w.health.SetReady(false)
				w.logger.Error("failed to mark webhook delivery delivered", "delivery_id", delivery.ID, "error", markErr)
			}
			continue
		}
		w.metrics.Fail()
		w.metrics.ObserveAge(time.Since(started))

		retryAt := time.Now().UTC().Add(webhookRetryDelay(delivery.Attempts))
		if markErr := w.repo.MarkFailed(ctx, delivery.ID, err.Error(), retryAt); markErr != nil {
			w.health.SetReady(false)
			w.logger.Error("failed to update webhook delivery failure", "delivery_id", delivery.ID, "error", markErr)
		}
		w.logger.Warn("webhook delivery failed", "delivery_id", delivery.ID, "attempt", delivery.Attempts, "error", err)
	}
}

func webhookRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}
