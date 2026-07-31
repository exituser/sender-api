package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisQueue struct {
	client *redis.Client
}

const (
	emailPendingKey    = "emails:pending"
	emailProcessingKey = "emails:processing"
	emailDeadKey       = "emails:dead"
	maxEmailAttempts   = 5
)

func NewRedisQueue(client *redis.Client) *RedisQueue {
	return &RedisQueue{client: client}
}

func (q *RedisQueue) Enqueue(ctx context.Context, emailID string) error {
	return q.client.LPush(ctx, emailPendingKey, emailID).Err()
}

func (q *RedisQueue) Dequeue(ctx context.Context) (string, error) {
	result, err := q.client.BRPopLPush(ctx, emailPendingKey, emailProcessingKey, 0).Result()
	if err != nil {
		return "", fmt.Errorf("dequeue failed: %w", err)
	}
	if result == "" {
		return "", fmt.Errorf("empty queue item")
	}
	return result, nil
}

func (q *RedisQueue) Ack(ctx context.Context, emailID string) error {
	return q.client.LRem(ctx, emailProcessingKey, 1, emailID).Err()
}

func (q *RedisQueue) Requeue(ctx context.Context, emailID string, countAttempt bool) error {
	if err := q.client.LRem(ctx, emailProcessingKey, 1, emailID).Err(); err != nil {
		return fmt.Errorf("remove processing item: %w", err)
	}

	if countAttempt {
		attemptKey := "emails:attempts:" + emailID
		attempts, err := q.client.Incr(ctx, attemptKey).Result()
		if err != nil {
			return fmt.Errorf("count delivery attempt: %w", err)
		}
		_ = q.client.Expire(ctx, attemptKey, 24*time.Hour).Err()
		if attempts >= maxEmailAttempts {
			if err := q.client.LPush(ctx, emailDeadKey, emailID).Err(); err != nil {
				return fmt.Errorf("move email to dead letter queue: %w", err)
			}
			return nil
		}
	}

	return q.client.RPush(ctx, emailPendingKey, emailID).Err()
}

func (q *RedisQueue) Recover(ctx context.Context) error {
	items, err := q.client.LRange(ctx, emailProcessingKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("list processing items: %w", err)
	}
	for _, emailID := range items {
		if err := q.client.LRem(ctx, emailProcessingKey, 1, emailID).Err(); err != nil {
			return fmt.Errorf("recover processing item: %w", err)
		}
		if err := q.client.RPush(ctx, emailPendingKey, emailID).Err(); err != nil {
			return fmt.Errorf("requeue recovered item: %w", err)
		}
	}
	return nil
}

func (q *RedisQueue) EnqueueInbound(ctx context.Context, messageID string) error {
	return q.client.LPush(ctx, "inbound:pending", messageID).Err()
}

func (q *RedisQueue) DequeueInbound(ctx context.Context) (string, error) {
	result, err := q.client.BRPop(ctx, 0, "inbound:pending").Result()
	if err != nil {
		return "", fmt.Errorf("inbound dequeue failed: %w", err)
	}
	if len(result) < 2 {
		return "", fmt.Errorf("unexpected result length")
	}
	return result[1], nil
}
