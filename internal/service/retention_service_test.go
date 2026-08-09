package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type retentionWebhookRepoStub struct {
	calls  []string
	before map[string]time.Time
	err    error
}

func (s *retentionWebhookRepoStub) PurgeByEventClass(_ context.Context, class string, before time.Time) (int64, error) {
	s.calls = append(s.calls, class)
	if s.before == nil {
		s.before = make(map[string]time.Time)
	}
	s.before[class] = before
	return 1, s.err
}

type retentionEmailRepoStub struct {
	calls  int
	before time.Time
}

func (s *retentionEmailRepoStub) PurgeEmailsBefore(_ context.Context, before time.Time) (int64, error) {
	s.calls++
	s.before = before
	return 2, nil
}

type retentionInboundRepoStub struct {
	listCalls   int
	deleteCalls int
	before      time.Time
	items       []domain.ExpiredInboundRecord
}

func (s *retentionInboundRepoStub) ListExpired(_ context.Context, before time.Time, _ int) ([]domain.ExpiredInboundRecord, error) {
	s.listCalls++
	s.before = before
	if s.items == nil {
		return []domain.ExpiredInboundRecord{{ID: uuid.New()}}, nil
	}
	return append([]domain.ExpiredInboundRecord(nil), s.items...), nil
}

func (s *retentionInboundRepoStub) DeleteExpired(_ context.Context, _ uuid.UUID, _ time.Time) (bool, error) {
	s.deleteCalls++
	return true, nil
}

type retentionObjectDeleterStub struct {
	keys []string
	err  error
}

func (s *retentionObjectDeleterStub) DeleteInboundObject(_ context.Context, key string) error {
	s.keys = append(s.keys, key)
	return s.err
}

func TestRetentionServicePurgesConfiguredData(t *testing.T) {
	emails := &retentionEmailRepoStub{}
	inbound := &retentionInboundRepoStub{}
	webhooks := &retentionWebhookRepoStub{}
	service := NewRetentionService(emails, inbound, webhooks)
	if err := service.Purge(context.Background(), 90*24*time.Hour, 30*24*time.Hour); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	if emails.calls != 1 || inbound.listCalls != 1 || inbound.deleteCalls != 1 {
		t.Fatalf("expected both repositories to be purged, emails=%d inbound_list=%d inbound_delete=%d", emails.calls, inbound.listCalls, inbound.deleteCalls)
	}
	if len(webhooks.calls) != 2 || webhooks.calls[0] != "outbound" || webhooks.calls[1] != "inbound" {
		t.Fatalf("expected outbound and inbound webhook purges, got %v", webhooks.calls)
	}
	if time.Since(emails.before.Add(90*24*time.Hour)) > time.Minute || time.Since(inbound.before.Add(30*24*time.Hour)) > time.Minute {
		t.Fatal("retention cutoff was not calculated from the current time")
	}
}

func TestRetentionDeletesRawObjectBeforeInboundDatabaseRow(t *testing.T) {
	id := uuid.New()
	inbound := &retentionInboundRepoStub{items: []domain.ExpiredInboundRecord{{ID: id, RawObjectKey: "raw/message.eml"}}}
	objects := &retentionObjectDeleterStub{}
	service := NewRetentionService(nil, inbound)
	service.SetInboundObjectDeleter(objects)
	if err := service.Purge(context.Background(), 0, 24*time.Hour); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	if len(objects.keys) != 1 || objects.keys[0] != "raw/message.eml" || inbound.deleteCalls != 1 {
		t.Fatalf("raw object and row were not removed together: keys=%v deletes=%d", objects.keys, inbound.deleteCalls)
	}
}

func TestRetentionKeepsInboundRowWhenRawObjectDeletionFails(t *testing.T) {
	want := errors.New("s3 unavailable")
	inbound := &retentionInboundRepoStub{items: []domain.ExpiredInboundRecord{{ID: uuid.New(), RawObjectKey: "raw/message.eml"}}}
	service := NewRetentionService(nil, inbound)
	service.SetInboundObjectDeleter(&retentionObjectDeleterStub{err: want})
	if err := service.Purge(context.Background(), 0, 24*time.Hour); !errors.Is(err, want) {
		t.Fatalf("Purge() error = %v, want wrapped %v", err, want)
	}
	if inbound.deleteCalls != 0 {
		t.Fatal("inbound database row was deleted before its raw object")
	}
}

func TestRetentionServiceReportsWebhookPurgeErrors(t *testing.T) {
	want := errors.New("database unavailable")
	emails := &retentionEmailRepoStub{}
	service := NewRetentionService(emails, nil, &retentionWebhookRepoStub{err: want})
	if err := service.Purge(context.Background(), 24*time.Hour, 0); !errors.Is(err, want) {
		t.Fatalf("Purge() error = %v, want wrapped %v", err, want)
	}
	if emails.calls != 0 {
		t.Fatal("source email rows must not be deleted while a retained payload copy remains")
	}
}

func TestRetentionServiceCanBeDisabled(t *testing.T) {
	service := NewRetentionService(&retentionEmailRepoStub{}, &retentionInboundRepoStub{})
	if err := service.Purge(context.Background(), 0, 0); err == nil {
		t.Fatal("expected disabled retention to return an error")
	}
}
