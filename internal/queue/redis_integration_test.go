//go:build integration

package queue

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sender-api/sender-api/internal/config"
	"github.com/sender-api/sender-api/internal/domain"
)

func integrationRedis(t *testing.T) (*redis.Client, *RedisQueue) {
	t.Helper()
	raw := os.Getenv("REDIS_TEST_URL")
	if raw == "" {
		t.Skip("REDIS_TEST_URL is not set")
	}
	options, err := config.ParseRedisOptions(raw)
	if err != nil {
		t.Fatalf("parse Redis URL: %v", err)
	}
	client := redis.NewClient(options)
	if err := client.Ping(context.Background()).Err(); err != nil {
		client.Close()
		t.Fatalf("connect Redis: %v", err)
	}
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		client.Close()
		t.Fatalf("flush test Redis database: %v", err)
	}
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})
	return client, NewRedisQueue(client)
}

func TestRedisQueueReceiptOwnershipAndExpiredRecoveryIntegration(t *testing.T) {
	client, queue := integrationRedis(t)
	ctx := context.Background()
	emailID := uuid.NewString()
	if err := queue.Enqueue(ctx, emailID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	receipt, err := queue.Dequeue(ctx)
	if err != nil || receipt == nil || receipt.EmailID != emailID {
		t.Fatalf("dequeue receipt=%+v err=%v", receipt, err)
	}
	processing, err := client.LRange(ctx, emailProcessingKey, 0, -1).Result()
	if err != nil || len(processing) != 1 || processing[0] != encodeReceipt(receipt) {
		t.Fatalf("processing receipt was not atomically tokenized: items=%v err=%v", processing, err)
	}

	wrong := &domain.QueueReceipt{EmailID: receipt.EmailID, Token: uuid.NewString(), LeaseUntil: receipt.LeaseUntil}
	if err := queue.Ack(ctx, wrong); err == nil {
		t.Fatal("wrong worker token acknowledged a live receipt")
	}
	if err := queue.Recover(ctx); err != nil {
		t.Fatalf("recover live receipt: %v", err)
	}
	if pending := client.LLen(ctx, emailPendingKey).Val(); pending != 0 {
		t.Fatalf("live receipt was recovered early: pending=%d", pending)
	}

	if err := client.ZAdd(ctx, emailLeasesKey, redis.Z{Score: float64(time.Now().Add(-time.Second).Unix()), Member: encodeReceipt(receipt)}).Err(); err != nil {
		t.Fatalf("expire receipt: %v", err)
	}
	if err := queue.Recover(ctx); err != nil {
		t.Fatalf("recover expired receipt: %v", err)
	}
	if pending := client.LLen(ctx, emailPendingKey).Val(); pending != 1 {
		t.Fatalf("expired receipt was not recovered: pending=%d", pending)
	}
	newReceipt, err := queue.Dequeue(ctx)
	if err != nil || newReceipt == nil || newReceipt.Token == receipt.Token {
		t.Fatalf("expected a new ownership token: receipt=%+v err=%v", newReceipt, err)
	}
	if err := queue.Ack(ctx, receipt); err == nil {
		t.Fatal("expired owner acknowledged the replacement receipt")
	}
	if err := queue.Ack(ctx, newReceipt); err != nil {
		t.Fatalf("new owner could not acknowledge receipt: %v", err)
	}
}

func TestRedisQueueLegacyReceiptGetsGraceLeaseIntegration(t *testing.T) {
	client, queue := integrationRedis(t)
	ctx := context.Background()
	emailID := uuid.NewString()
	if err := client.LPush(ctx, emailProcessingKey, emailID).Err(); err != nil {
		t.Fatalf("seed legacy receipt: %v", err)
	}
	if err := queue.Recover(ctx); err != nil {
		t.Fatalf("first legacy recovery: %v", err)
	}
	if client.LLen(ctx, emailPendingKey).Val() != 0 || client.LLen(ctx, emailProcessingKey).Val() != 1 {
		t.Fatal("legacy live receipt was stolen during rolling recovery")
	}
	if err := client.ZAdd(ctx, emailLeasesKey, redis.Z{Score: float64(time.Now().Add(-time.Second).Unix()), Member: emailID}).Err(); err != nil {
		t.Fatalf("expire legacy receipt: %v", err)
	}
	if err := queue.Recover(ctx); err != nil {
		t.Fatalf("second legacy recovery: %v", err)
	}
	if client.LLen(ctx, emailPendingKey).Val() != 1 || client.LLen(ctx, emailProcessingKey).Val() != 0 {
		t.Fatal("expired legacy receipt was not recovered")
	}
}

func TestRedisDeadLetterReplayIsMembershipCheckedIntegration(t *testing.T) {
	client, queue := integrationRedis(t)
	ctx := context.Background()
	emailID := uuid.NewString()
	if err := client.LPush(ctx, emailDeadKey, emailID).Err(); err != nil {
		t.Fatalf("seed dead letter: %v", err)
	}
	if err := queue.ReplayDead(ctx, emailID); err != nil {
		t.Fatalf("replay dead letter: %v", err)
	}
	if client.LLen(ctx, emailDeadKey).Val() != 0 || client.LLen(ctx, emailPendingKey).Val() != 1 {
		t.Fatal("dead letter was not atomically moved back to pending")
	}
	if err := queue.ReplayDead(ctx, emailID); !errors.Is(err, domain.ErrDeadLetterNotFound) {
		t.Fatalf("missing dead letter error = %v", err)
	}
}
