package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/internal/queue"
	"github.com/sender-api/sender-api/internal/service"
	"github.com/sender-api/sender-api/pkg/metrics"
)

type EmailWorker struct {
	emailService  *service.EmailService
	queue         domain.EmailQueue
	logger        *slog.Logger
	pollInterval  time.Duration
	metrics       *metrics.WorkerMetrics
	health        *HealthState
	recoveryReady atomic.Bool
}

func NewEmailWorker(emailService *service.EmailService, queue domain.EmailQueue, logger *slog.Logger, pollInterval time.Duration) *EmailWorker {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &EmailWorker{
		emailService: emailService,
		queue:        queue,
		logger:       logger,
		pollInterval: pollInterval,
		metrics:      metrics.NewWorkerMetrics("email"),
		health:       NewHealthState(),
	}
}

func (w *EmailWorker) Health() *HealthState { return w.health }

func (w *EmailWorker) Start(ctx context.Context) {
	defer w.health.SetReady(false)
	w.logger.Info("email worker started")
	ready := true
	if err := w.emailService.RecoverSending(ctx); err != nil {
		ready = false
		w.logger.Error("failed to recover sending emails", "error", err)
	}
	if err := w.queue.Recover(ctx); err != nil {
		ready = false
		w.logger.Error("failed to recover processing emails", "error", err)
	}
	w.health.SetReady(ready)
	w.recoveryReady.Store(ready)

	lastPromotion := time.Time{}
	maintenance := time.NewTicker(30 * time.Second)
	defer maintenance.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("email worker stopped")
			return
		case <-maintenance.C:
			maintenanceReady := true
			if err := w.emailService.RecoverSending(ctx); err != nil {
				maintenanceReady = false
				w.logger.Error("failed to recover stuck sending emails", "error", err)
			}
			if err := w.queue.Recover(ctx); err != nil {
				maintenanceReady = false
				w.logger.Error("failed to recover expired queue leases", "error", err)
			}
			w.health.SetReady(maintenanceReady)
			w.recoveryReady.Store(maintenanceReady)
		default:
			w.health.Heartbeat()
			iterationReady := w.recoveryReady.Load()
			if lastPromotion.IsZero() || time.Since(lastPromotion) >= w.pollInterval {
				if err := w.queue.PromoteScheduled(ctx); err != nil {
					iterationReady = false
					w.logger.Error("failed to promote scheduled emails", "error", err)
				}
				lastPromotion = time.Now()
			}
			if !w.processNext(ctx) {
				iterationReady = false
			}
			w.health.SetReady(iterationReady)
		}
	}
}

func (w *EmailWorker) processNext(ctx context.Context) bool {
	receipt, err := w.queue.Dequeue(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		w.logger.Error("failed to dequeue email", "error", err)
		sleepContext(ctx, time.Second)
		return false
	}

	if receipt == nil || receipt.EmailID == "" {
		sleepContext(ctx, w.pollInterval)
		return true
	}
	emailID := receipt.EmailID
	w.metrics.Start()

	w.logger.Info("processing email", "email_id", emailID)

	if err := w.emailService.ProcessFromQueue(ctx, emailID); err != nil {
		w.metrics.Fail()
		w.logger.Error("failed to process email", "email_id", emailID, "error", err)
		if errors.Is(err, service.ErrEmailDeliveryFailed) || errors.Is(err, service.ErrEmailNotQueued) ||
			errors.Is(err, service.ErrEmailAccepted) || errors.Is(err, service.ErrEmailOutcomeAmbiguous) {
			if ackErr := w.queue.Ack(ctx, receipt); ackErr != nil {
				w.logger.Error("failed to acknowledge terminal email", "email_id", emailID, "error", ackErr)
				return false
			}
			return true
		}
		if errors.Is(err, service.ErrEmailNotDue) {
			var notDue *service.EmailNotDueError
			if errors.As(err, &notDue) {
				if rescheduleErr := w.reschedule(ctx, receipt, notDue.At); rescheduleErr != nil {
					w.logger.Error("failed to reschedule email", "email_id", emailID, "error", rescheduleErr)
					if fallbackErr := w.queue.Requeue(ctx, receipt, false); fallbackErr != nil {
						w.logger.Error("failed to requeue scheduled email", "email_id", emailID, "error", fallbackErr)
						return false
					}
				}
				return true
			}
		}
		countAttempt := !errors.Is(err, service.ErrEmailNotDue)
		if requeueErr := w.queue.Requeue(ctx, receipt, countAttempt); requeueErr != nil {
			if errors.Is(requeueErr, queue.ErrDeadLettered) {
				if markErr := w.emailService.MarkFailedFromQueue(ctx, emailID, "delivery retry limit exceeded"); markErr != nil {
					w.logger.Error("failed to mark dead-lettered email failed", "email_id", emailID, "error", markErr)
					return false
				}
				return true
			}
			w.logger.Error("failed to requeue email", "email_id", emailID, "error", requeueErr)
			return false
		}
		sleepContext(ctx, time.Second)
		return true
	}

	if err := w.queue.Ack(ctx, receipt); err != nil {
		w.metrics.Fail()
		w.logger.Error("failed to acknowledge email", "email_id", emailID, "error", err)
		return false
	}
	w.metrics.Complete()

	w.logger.Info("email processed successfully", "email_id", emailID)
	return true
}

type receiptRescheduler interface {
	RescheduleReceipt(context.Context, *domain.QueueReceipt, time.Time) error
}

func (w *EmailWorker) reschedule(ctx context.Context, receipt *domain.QueueReceipt, at time.Time) error {
	if rescheduler, ok := w.queue.(receiptRescheduler); ok {
		return rescheduler.RescheduleReceipt(ctx, receipt, at)
	}
	return w.queue.Reschedule(ctx, receipt.EmailID, at)
}

func sleepContext(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
