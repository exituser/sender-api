package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/sender-api/sender-api/internal/service"
)

type RetentionWorker struct {
	service    *service.RetentionService
	emailAge   time.Duration
	inboundAge time.Duration
	logger     *slog.Logger
}

func NewRetentionWorker(retentionService *service.RetentionService, emailAge, inboundAge time.Duration, logger *slog.Logger) *RetentionWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &RetentionWorker{service: retentionService, emailAge: emailAge, inboundAge: inboundAge, logger: logger}
}

func (w *RetentionWorker) Start(ctx context.Context) {
	if w.service == nil || (w.emailAge <= 0 && w.inboundAge <= 0) {
		w.logger.Info("retention worker disabled")
		return
	}
	w.run(ctx)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.run(ctx)
		}
	}
}

func (w *RetentionWorker) run(ctx context.Context) {
	if err := w.service.Purge(ctx, w.emailAge, w.inboundAge); err != nil {
		w.logger.Error("retention purge failed", "error", err)
		return
	}
	w.logger.Info("retention purge completed", "email_age", w.emailAge, "inbound_age", w.inboundAge)
}
