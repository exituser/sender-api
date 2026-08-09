package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/metrics"
)

type RedisQueue struct {
	client *redis.Client
}

const (
	emailPendingKey    = "emails:pending"
	emailProcessingKey = "emails:processing"
	emailScheduledKey  = "emails:scheduled"
	emailDeadKey       = "emails:dead"
	emailLeasesKey     = "emails:leases"
	maxEmailAttempts   = 5
	emailLeaseDuration = 5 * time.Minute
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

func (q *RedisQueue) Dequeue(ctx context.Context) (*domain.QueueReceipt, error) {
	token := uuid.NewString()
	leaseUntil := time.Now().UTC().Add(emailLeaseDuration)
	const script = `
		local email_id = redis.call('RPOP', KEYS[1])
		if not email_id then return nil end
		local receipt = ARGV[1] .. '|' .. email_id
		redis.call('LPUSH', KEYS[2], receipt)
		redis.call('ZADD', KEYS[3], ARGV[2], receipt)
		return email_id
	`
	result, err := q.client.Eval(
		ctx,
		script,
		[]string{emailPendingKey, emailProcessingKey, emailLeasesKey},
		token,
		leaseUntil.Unix(),
	).Text()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue failed: %w", err)
	}
	if result == "" {
		return nil, fmt.Errorf("empty queue item")
	}

	return &domain.QueueReceipt{EmailID: result, Token: token, LeaseUntil: leaseUntil}, nil
}

func (q *RedisQueue) Ack(ctx context.Context, receipt *domain.QueueReceipt) error {
	if receipt == nil {
		return fmt.Errorf("ack requires a queue receipt")
	}
	const script = `
		local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
		if removed > 0 then redis.call('ZREM', KEYS[2], ARGV[1]) end
		return removed
	`
	removed, err := q.client.Eval(ctx, script, []string{emailProcessingKey, emailLeasesKey}, encodeReceipt(receipt)).Int()
	if err != nil {
		return err
	}
	if removed != 1 {
		return fmt.Errorf("queue receipt is no longer owned")
	}
	return nil
}

func (q *RedisQueue) Requeue(ctx context.Context, receipt *domain.QueueReceipt, countAttempt bool) error {
	if receipt == nil {
		return fmt.Errorf("requeue requires a queue receipt")
	}
	const script = `
		local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
		if removed == 0 then return -1 end
		redis.call('ZREM', KEYS[5], ARGV[1])
		local attempts = 0
		if ARGV[3] == '1' then
			attempts = redis.call('INCR', KEYS[3])
			redis.call('EXPIRE', KEYS[3], 86400)
			if attempts >= tonumber(ARGV[4]) then
				redis.call('LPUSH', KEYS[2], ARGV[5])
				redis.call('LTRIM', KEYS[2], 0, 9999)
				return 1
			end
		end
		local delay = 1
		if attempts > 1 then delay = 2 ^ (attempts - 1) end
		if delay > 300 then delay = 300 end
		redis.call('ZADD', KEYS[4], tonumber(ARGV[2]) + delay, ARGV[5])
		return 0
	`
	attemptKey := "emails:attempts:" + receipt.EmailID
	result, err := q.client.Eval(ctx, script,
		[]string{emailProcessingKey, emailDeadKey, attemptKey, emailScheduledKey, emailLeasesKey},
		encodeReceipt(receipt), time.Now().Unix(), boolString(countAttempt), maxEmailAttempts, receipt.EmailID,
	).Int()
	if err != nil {
		return fmt.Errorf("requeue email: %w", err)
	}
	if result == 1 {
		return ErrDeadLettered
	}
	if result == -1 {
		return fmt.Errorf("queue receipt is no longer owned")
	}
	return nil
}

// RescheduleReceipt releases exactly this lease before putting the email back
// into the scheduled queue. It is intentionally separate from the legacy
// ID-based Reschedule method in the domain interface.
func (q *RedisQueue) RescheduleReceipt(ctx context.Context, receipt *domain.QueueReceipt, at time.Time) error {
	if receipt == nil {
		return fmt.Errorf("reschedule requires a queue receipt")
	}
	const script = `
		local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
		if removed == 0 then return 0 end
		redis.call('ZREM', KEYS[3], ARGV[1])
		redis.call('ZADD', KEYS[2], ARGV[2], ARGV[3])
		return 1
	`
	result, err := q.client.Eval(ctx, script, []string{emailProcessingKey, emailScheduledKey, emailLeasesKey}, encodeReceipt(receipt), at.Unix(), receipt.EmailID).Int()
	if err != nil {
		return fmt.Errorf("reschedule email: %w", err)
	}
	if result == 0 {
		return fmt.Errorf("queue receipt is no longer owned")
	}
	return nil
}

func (q *RedisQueue) ListDead(ctx context.Context, limit int) ([]string, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return q.client.LRange(ctx, emailDeadKey, 0, int64(limit-1)).Result()
}

func (q *RedisQueue) ReplayDead(ctx context.Context, emailID string) error {
	const script = `
		local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
		if removed == 0 then return 0 end
		redis.call('DEL', KEYS[2])
		redis.call('LPUSH', KEYS[3], ARGV[1])
		return 1
	`
	attemptKey := "emails:attempts:" + emailID
	result, err := q.client.Eval(ctx, script, []string{emailDeadKey, attemptKey, emailPendingKey}, emailID).Int()
	if err != nil {
		return fmt.Errorf("replay dead letter: %w", err)
	}
	if result == 0 {
		return fmt.Errorf("%w: %s", domain.ErrDeadLetterNotFound, emailID)
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
	const script = `
		local items = redis.call('LRANGE', KEYS[1], 0, -1)
		redis.call('DEL', KEYS[1])
		local recovered = 0
		local keep = {}
		for _, item in ipairs(items) do
			local separator = string.find(item, '|', 1, true)
			local lease = redis.call('ZSCORE', KEYS[3], item)
			if separator and lease and tonumber(lease) <= tonumber(ARGV[1]) then
				redis.call('RPUSH', KEYS[2], string.sub(item, separator + 1))
				redis.call('ZREM', KEYS[3], item)
				recovered = recovered + 1
			elseif not separator and string.match(item, '^[0-9a-fA-F]+%-[0-9a-fA-F]+%-[0-9a-fA-F]+%-[0-9a-fA-F]+%-[0-9a-fA-F]+$') then
				-- A rolling deployment can leave a raw receipt owned by an old
				-- worker. Give it one full lease before recovery instead of
				-- stealing live work during startup.
				if lease and tonumber(lease) <= tonumber(ARGV[1]) then
					redis.call('RPUSH', KEYS[2], item)
					redis.call('ZREM', KEYS[3], item)
					recovered = recovered + 1
				else
					if not lease then
						redis.call('ZADD', KEYS[3], tonumber(ARGV[1]) + tonumber(ARGV[2]), item)
					end
					table.insert(keep, item)
				end
			else
				table.insert(keep, item)
			end
		end
		for _, item in ipairs(keep) do redis.call('RPUSH', KEYS[1], item) end
		return recovered
	`
	recovered, err := q.client.Eval(ctx, script, []string{emailProcessingKey, emailPendingKey, emailLeasesKey}, time.Now().Unix(), int64(emailLeaseDuration/time.Second)).Int64()
	if err != nil {
		return fmt.Errorf("recover processing items: %w", err)
	}
	metrics.AddCounter("sender_api_queue_lease_recovered_total", uint64(recovered))
	q.observeDepths(ctx)
	return nil
}

func (q *RedisQueue) observeDepths(ctx context.Context) {
	pipeline := q.client.Pipeline()
	pending := pipeline.LLen(ctx, emailPendingKey)
	processing := pipeline.LLen(ctx, emailProcessingKey)
	dead := pipeline.LLen(ctx, emailDeadKey)
	scheduled := pipeline.ZCard(ctx, emailScheduledKey)
	if _, err := pipeline.Exec(ctx); err != nil {
		return
	}
	metrics.SetGauge("sender_api_queue_pending", pending.Val())
	metrics.SetGauge("sender_api_queue_processing", processing.Val())
	metrics.SetGauge("sender_api_queue_dead", dead.Val())
	metrics.SetGauge("sender_api_queue_scheduled", scheduled.Val())
}

func encodeReceipt(receipt *domain.QueueReceipt) string {
	return receipt.Token + "|" + receipt.EmailID
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
