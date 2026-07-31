package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const usageKeyPrefix = "usage:email-recipients:"

type RedisUsageLimiter struct {
	client *redis.Client
}

func NewRedisUsageLimiter(client *redis.Client) *RedisUsageLimiter {
	return &RedisUsageLimiter{client: client}
}

func (l *RedisUsageLimiter) Reserve(ctx context.Context, teamID uuid.UUID, units, limit int) (bool, error) {
	if l == nil || l.client == nil || units <= 0 || limit <= 0 {
		return true, nil
	}
	key, expiresAt := usageKey(teamID, time.Now().UTC())
	const script = `
		local current = redis.call('INCRBY', KEYS[1], ARGV[1])
		if current == tonumber(ARGV[1]) then
			redis.call('EXPIREAT', KEYS[1], ARGV[3])
		end
		if current > tonumber(ARGV[2]) then
			redis.call('DECRBY', KEYS[1], ARGV[1])
			return 0
		end
		return 1
	`
	result, err := l.client.Eval(ctx, script, []string{key}, units, limit, expiresAt.Unix()).Int()
	if err != nil {
		return false, fmt.Errorf("reserve daily email quota: %w", err)
	}
	return result == 1, nil
}

func (l *RedisUsageLimiter) Release(ctx context.Context, teamID uuid.UUID, units int) error {
	if l == nil || l.client == nil || units <= 0 {
		return nil
	}
	key, _ := usageKey(teamID, time.Now().UTC())
	const script = `
		local current = redis.call('GET', KEYS[1])
		if not current then return 0 end
		local next = math.max(0, tonumber(current) - tonumber(ARGV[1]))
		redis.call('SET', KEYS[1], next, 'KEEPTTL')
		return next
	`
	if _, err := l.client.Eval(ctx, script, []string{key}, units).Result(); err != nil {
		return fmt.Errorf("release daily email quota: %w", err)
	}
	return nil
}

func usageKey(teamID uuid.UUID, now time.Time) (string, time.Time) {
	startOfTomorrow := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	return usageKeyPrefix + teamID.String() + ":" + now.Format("2006-01-02"), startOfTomorrow
}
