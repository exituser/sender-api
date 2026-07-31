package worker

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/webhook"
)

type WebhookWorker struct {
	repo         domain.WebhookDeliveryRepository
	logger       *slog.Logger
	pollInterval time.Duration
}

func NewWebhookWorker(repo domain.WebhookDeliveryRepository, logger *slog.Logger, pollInterval time.Duration) *WebhookWorker {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &WebhookWorker{repo: repo, logger: logger, pollInterval: pollInterval}
}

func (w *WebhookWorker) Start(ctx context.Context) {
	w.logger.Info("webhook worker started")
	if err := w.repo.RecoverStale(ctx); err != nil {
		w.logger.Error("failed to recover stale webhook deliveries", "error", err)
	}
	maintenance := time.NewTicker(30 * time.Second)
	defer maintenance.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("webhook worker stopped")
			return
		case <-maintenance.C:
			if err := w.repo.RecoverStale(ctx); err != nil {
				w.logger.Error("failed to recover stale webhook deliveries", "error", err)
			}
		default:
		}

		delivery, err := w.repo.ClaimDelivery(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			sleepContext(ctx, w.pollInterval)
			continue
		}
		if err != nil {
			w.logger.Error("failed to claim webhook delivery", "error", err)
			sleepContext(ctx, w.pollInterval)
			continue
		}

		deliveryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = webhook.SendWebhookWithID(deliveryCtx, delivery.ID, delivery.URL, delivery.Secret, delivery.Event, delivery.Payload)
		cancel()
		if err == nil {
			if markErr := w.repo.MarkDelivered(ctx, delivery.ID); markErr != nil {
				w.logger.Error("failed to mark webhook delivery delivered", "delivery_id", delivery.ID, "error", markErr)
			}
			continue
		}

		retryAt := time.Now().UTC().Add(webhookRetryDelay(delivery.Attempts))
		if markErr := w.repo.MarkFailed(ctx, delivery.ID, err.Error(), retryAt); markErr != nil {
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
