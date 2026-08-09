package worker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sender-api/sender-api/internal/service"
)

func TestRetentionWorkerHeartbeatsBetweenRuns(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	retentionService := service.NewRetentionService(nil, nil)
	retentionWorker := NewRetentionWorker(retentionService, time.Hour, 0, logger)
	retentionWorker.runInterval = time.Hour
	retentionWorker.heartbeatInterval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		retentionWorker.Start(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for !retentionWorker.Health().Healthy(time.Now(), time.Second) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !retentionWorker.Health().Healthy(time.Now(), time.Second) {
		cancel()
		<-done
		t.Fatal("retention worker did not become ready after its initial purge")
	}

	previousHeartbeat := retentionWorker.Health().lastHeartbeat.Load()
	deadline = time.Now().Add(time.Second)
	for retentionWorker.Health().lastHeartbeat.Load() <= previousHeartbeat && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if retentionWorker.Health().lastHeartbeat.Load() <= previousHeartbeat {
		cancel()
		<-done
		t.Fatal("retention worker did not refresh its heartbeat while waiting for the next purge")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retention worker did not stop after cancellation")
	}
}
