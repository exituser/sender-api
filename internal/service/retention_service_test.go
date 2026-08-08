package service

import (
	"context"
	"testing"
	"time"
)

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
	calls  int
	before time.Time
}

func (s *retentionInboundRepoStub) PurgeBefore(_ context.Context, before time.Time) (int64, error) {
	s.calls++
	s.before = before
	return 1, nil
}

func TestRetentionServicePurgesConfiguredData(t *testing.T) {
	emails := &retentionEmailRepoStub{}
	inbound := &retentionInboundRepoStub{}
	service := NewRetentionService(emails, inbound)
	if err := service.Purge(context.Background(), 90*24*time.Hour, 30*24*time.Hour); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	if emails.calls != 1 || inbound.calls != 1 {
		t.Fatalf("expected both repositories to be purged, emails=%d inbound=%d", emails.calls, inbound.calls)
	}
	if time.Since(emails.before.Add(90*24*time.Hour)) > time.Minute || time.Since(inbound.before.Add(30*24*time.Hour)) > time.Minute {
		t.Fatal("retention cutoff was not calculated from the current time")
	}
}

func TestRetentionServiceCanBeDisabled(t *testing.T) {
	service := NewRetentionService(&retentionEmailRepoStub{}, &retentionInboundRepoStub{})
	if err := service.Purge(context.Background(), 0, 0); err == nil {
		t.Fatal("expected disabled retention to return an error")
	}
}
