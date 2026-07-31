package queue

import (
	"context"
	"errors"
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
	emailScheduledKey  = "emails:scheduled"
	emailDeadKey       = "emails:dead"
	maxEmailAttempts   = 5
)

var ErrDeadLettered = errors.New("email moved to dead letter queue")

func NewRedisQueue(client *redis.Client) *RedisQueue {
	return &RedisQueue{client: client}
}

func (q *RedisQueue) Enqueue(ctx context.Context, emailID string) error {
	return q.client.LPush(ctx, emailPendingKey, emailID).Err()
}

func (q *RedisQueue) Schedule(ctx context.Context, emailID string, at time.Time) error {
	return q.client.ZAdd(ctx, emailScheduledKey, redis.Z{Score: float64(at.Unix()), Member: emailID}).Err()
}

func (q *RedisQueue) Reschedule(ctx context.Context, emailID string, at time.Time) error {
	const script = `
		redis.call('LREM', KEYS[1], 1, ARGV[1])
		redis.call('ZADD', KEYS[2], ARGV[2], ARGV[1])
		return 1
	`
	return q.client.Eval(ctx, script, []string{emailProcessingKey, emailScheduledKey}, emailID, at.Unix()).Err()
}

func (q *RedisQueue) PromoteScheduled(ctx context.Context) error {
	const script = `
		local ids = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, 100)
		for _, id in ipairs(ids) do
			redis.call('ZREM', KEYS[1], id)
			redis.call('LPUSH', KEYS[2], id)
		end
		return #ids
	`
	return q.client.Eval(ctx, script, []string{emailScheduledKey, emailPendingKey}, time.Now().Unix()).Err()
}

func (q *RedisQueue) Dequeue(ctx context.Context) (string, error) {
	result, err := q.client.BLMove(ctx, emailPendingKey, emailProcessingKey, "RIGHT", "LEFT", time.Second).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
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
	const script = `
		redis.call('LREM', KEYS[1], 1, ARGV[1])
		local attempts = 0
		if ARGV[3] == '1' then
			attempts = redis.call('INCR', KEYS[3])
			redis.call('EXPIRE', KEYS[3], 86400)
			if attempts >= tonumber(ARGV[4]) then
				redis.call('LPUSH', KEYS[2], ARGV[1])
				return 1
			end
		end
		local delay = 1
		if attempts > 1 then delay = 2 ^ (attempts - 1) end
		if delay > 300 then delay = 300 end
		redis.call('ZADD', KEYS[4], tonumber(ARGV[2]) + delay, ARGV[1])
		return 0
	`
	attemptKey := "emails:attempts:" + emailID
	result, err := q.client.Eval(ctx, script,
		[]string{emailProcessingKey, emailDeadKey, attemptKey, emailScheduledKey},
		emailID, time.Now().Unix(), boolString(countAttempt), maxEmailAttempts,
	).Int()
	if err != nil {
		return fmt.Errorf("requeue email: %w", err)
	}
	if result == 1 {
		return ErrDeadLettered
	}
	return nil
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
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
