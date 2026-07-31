package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/internal/queue"
	"github.com/sender-api/sender-api/internal/service"
)

type EmailWorker struct {
	emailService *service.EmailService
	queue        domain.EmailQueue
	logger       *slog.Logger
	pollInterval time.Duration
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
	}
}

func (w *EmailWorker) Start(ctx context.Context) {
	w.logger.Info("email worker started")
	if err := w.emailService.RecoverSending(ctx); err != nil {
		w.logger.Error("failed to recover sending emails", "error", err)
	}
	if err := w.queue.Recover(ctx); err != nil {
		w.logger.Error("failed to recover processing emails", "error", err)
	}

	lastPromotion := time.Time{}
	maintenance := time.NewTicker(30 * time.Second)
	defer maintenance.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("email worker stopped")
			return
		case <-maintenance.C:
			if err := w.emailService.RecoverSending(ctx); err != nil {
				w.logger.Error("failed to recover stuck sending emails", "error", err)
			}
		default:
			if lastPromotion.IsZero() || time.Since(lastPromotion) >= w.pollInterval {
				if err := w.queue.PromoteScheduled(ctx); err != nil {
					w.logger.Error("failed to promote scheduled emails", "error", err)
				}
				lastPromotion = time.Now()
			}
			w.processNext(ctx)
		}
	}
}

func (w *EmailWorker) processNext(ctx context.Context) {
	emailID, err := w.queue.Dequeue(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.logger.Error("failed to dequeue email", "error", err)
		sleepContext(ctx, time.Second)
		return
	}

	if emailID == "" {
		return
	}

	w.logger.Info("processing email", "email_id", emailID)

	if err := w.emailService.ProcessFromQueue(ctx, emailID); err != nil {
		w.logger.Error("failed to process email", "email_id", emailID, "error", err)
		if errors.Is(err, service.ErrEmailDeliveryFailed) || errors.Is(err, service.ErrEmailNotQueued) {
			if ackErr := w.queue.Ack(ctx, emailID); ackErr != nil {
				w.logger.Error("failed to acknowledge terminal email", "email_id", emailID, "error", ackErr)
			}
			return
		}
		if errors.Is(err, service.ErrEmailNotDue) {
			var notDue *service.EmailNotDueError
			if errors.As(err, &notDue) {
				if rescheduleErr := w.queue.Reschedule(ctx, emailID, notDue.At); rescheduleErr != nil {
					w.logger.Error("failed to reschedule email", "email_id", emailID, "error", rescheduleErr)
					if fallbackErr := w.queue.Requeue(ctx, emailID, false); fallbackErr != nil {
						w.logger.Error("failed to requeue scheduled email", "email_id", emailID, "error", fallbackErr)
					}
				}
				return
			}
		}
		countAttempt := !errors.Is(err, service.ErrEmailNotDue)
		if requeueErr := w.queue.Requeue(ctx, emailID, countAttempt); requeueErr != nil {
			if errors.Is(requeueErr, queue.ErrDeadLettered) {
				if markErr := w.emailService.MarkFailedFromQueue(ctx, emailID, "delivery retry limit exceeded"); markErr != nil {
					w.logger.Error("failed to mark dead-lettered email failed", "email_id", emailID, "error", markErr)
				}
				return
			}
			w.logger.Error("failed to requeue email", "email_id", emailID, "error", requeueErr)
		}
		sleepContext(ctx, time.Second)
		return
	}

	if err := w.queue.Ack(ctx, emailID); err != nil {
		w.logger.Error("failed to acknowledge email", "email_id", emailID, "error", err)
		return
	}

	w.logger.Info("email processed successfully", "email_id", emailID)
}

func sleepContext(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
