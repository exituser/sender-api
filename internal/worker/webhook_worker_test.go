package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sender-api/sender-api/internal/domain"
)

type boundedWebhookRepo struct {
	mu         sync.Mutex
	deliveries []*domain.WebhookDelivery
	delivered  atomic.Int64
	failed     atomic.Int64
	active     atomic.Int64
	maxActive  atomic.Int64
	recoverErr error
}

func (r *boundedWebhookRepo) CreateDelivery(context.Context, *domain.WebhookDelivery) error {
	return nil
}
func (r *boundedWebhookRepo) RecoverStale(context.Context) error { return r.recoverErr }
func (r *boundedWebhookRepo) ClaimDelivery(context.Context) (*domain.WebhookDelivery, error) {
	current := r.active.Add(1)
	for {
		old := r.maxActive.Load()
		if current <= old || r.maxActive.CompareAndSwap(old, current) {
			break
		}
	}
	defer r.active.Add(-1)
	time.Sleep(10 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.deliveries) == 0 {
		return nil, pgx.ErrNoRows
	}
	delivery := r.deliveries[0]
	r.deliveries = r.deliveries[1:]
	return delivery, nil
}
func (r *boundedWebhookRepo) MarkDelivered(context.Context, uuid.UUID) error {
	r.delivered.Add(1)
	return nil
}
func (r *boundedWebhookRepo) MarkFailed(context.Context, uuid.UUID, string, time.Time) error {
	r.failed.Add(1)
	return nil
}
func (r *boundedWebhookRepo) ReplayFailed(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func TestWebhookWorkerHonorsConcurrencyLimit(t *testing.T) {
	repo := &boundedWebhookRepo{}
	for i := 0; i < 8; i++ {
		repo.deliveries = append(repo.deliveries, &domain.WebhookDelivery{ID: uuid.New(), URL: "not-a-url", Event: "test", Payload: []byte(`{"ok":true}`), Attempts: 1})
	}
	worker := NewWebhookWorkerWithOptions(repo, nil, time.Millisecond, WithWebhookConcurrency(2))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	worker.Start(ctx)
	if got := repo.maxActive.Load(); got > 2 {
		t.Fatalf("max concurrent claims = %d, want <= 2", got)
	}
	if got := repo.failed.Load(); got != 8 {
		t.Fatalf("failed = %d, want 8", got)
	}
}

func TestWebhookWorkerDoesNotMaskFailedRecoveryWithHeartbeat(t *testing.T) {
	repo := &boundedWebhookRepo{recoverErr: errors.New("recovery query failed")}
	worker := NewWebhookWorkerWithOptions(repo, nil, time.Millisecond, WithWebhookConcurrency(1))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()
	time.Sleep(25 * time.Millisecond)
	if worker.Health().Healthy(time.Now(), time.Minute) {
		t.Fatal("worker reported ready after its recovery step failed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}
