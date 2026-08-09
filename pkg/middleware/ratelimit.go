package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sender-api/sender-api/internal/auth"
)

func RateLimit(redisClient *redis.Client, scopes ...string) func(http.Handler) http.Handler {
	return rateLimit(redisClient, rateLimitScope(scopes...), getLimitForPlan)
}

// RateLimitFixed is intended for authenticated provider callbacks whose burst
// profile is unrelated to a customer plan. Signature verification remains the
// trust boundary; this limit only bounds resource use before verification.
func RateLimitFixed(redisClient *redis.Client, limit int, scopes ...string) func(http.Handler) http.Handler {
	if limit < 1 {
		limit = 1
	}
	return rateLimit(redisClient, rateLimitScope(scopes...), func(context.Context) int { return limit })
}

func rateLimit(redisClient *redis.Client, scope string, limitForContext func(context.Context) int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redisClient == nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			key := getRateLimitKeyForScope(ctx, scope)
			limit := limitForContext(ctx)

			count, retryAfter, err := incrementRateLimit(ctx, redisClient, key, limit)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprint(w, `{"error":"rate limiter unavailable"}`)
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, limit-int(count))))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", retryAfter))

			if int(count) > limit {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", max(1, int(retryAfter))))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = fmt.Fprintf(w, `{"error":"rate limit exceeded"}`)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func getRateLimitKeyForScope(ctx context.Context, scope string) string {
	claims := auth.GetClaimsFromContext(ctx)
	prefix := "ratelimit:" + scope + ":"
	if claims == nil {
		return prefix + "anonymous"
	}
	if claims.TeamID != "" {
		return prefix + "team:" + claims.TeamID
	}
	if claims.UserID != "" {
		return prefix + "user:" + claims.UserID
	}
	return prefix + "anonymous"
}

func rateLimitScope(scopes ...string) string {
	if len(scopes) > 0 {
		if scope := strings.TrimSpace(scopes[0]); scope != "" {
			return scope
		}
	}
	return "api"
}

func incrementRateLimit(ctx context.Context, client *redis.Client, key string, limit int) (int64, int64, error) {
	const rateLimitWindowMilliseconds = 1000
	const script = `
		local now = tonumber(ARGV[1])
		local window = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
		local count = redis.call('ZCARD', KEYS[1])
		if count >= limit then
			local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
			local retry = window
			if #oldest >= 2 then retry = math.max(1, oldest[2] + window - now) end
			redis.call('PEXPIRE', KEYS[1], window + 1000)
			return {count + 1, math.ceil(retry / 1000)}
		end
		local sequence = redis.call('INCR', KEYS[2])
		redis.call('ZADD', KEYS[1], now, tostring(now) .. ':' .. tostring(sequence))
		redis.call('PEXPIRE', KEYS[1], window + 1000)
		redis.call('PEXPIRE', KEYS[2], window + 1000)
		return {count + 1, 0}
	`
	values, err := client.Eval(ctx, script, []string{key, key + ":sequence"}, time.Now().UnixMilli(), rateLimitWindowMilliseconds, limit).Int64Slice()
	if err != nil {
		return 0, 0, err
	}
	if len(values) != 2 {
		return 0, 0, fmt.Errorf("unexpected rate limiter response")
	}
	return values[0], values[1], nil
}

func getLimitForPlan(ctx context.Context) int {
	claims := auth.GetClaimsFromContext(ctx)
	if claims == nil || claims.TeamID == "" {
		return 10
	}
	switch claims.Plan {
	case "pro":
		return 50
	case "scale":
		return 200
	default:
		return 10
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
