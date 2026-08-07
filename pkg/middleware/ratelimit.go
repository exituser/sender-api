package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/sender-api/sender-api/internal/auth"
)

func RateLimit(redisClient *redis.Client, scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redisClient == nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			key := getRateLimitKeyForScope(ctx, rateLimitScope(scopes...))
			limit := getLimitForPlan(ctx)

			count, err := incrementRateLimit(ctx, redisClient, key)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprint(w, `{"error":"rate limiter unavailable"}`)
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, limit-int(count))))

			if int(count) > limit {
				w.Header().Set("Retry-After", "1")
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

func incrementRateLimit(ctx context.Context, client *redis.Client, key string) (int64, error) {
	const rateLimitWindowSeconds = 1
	const script = `
		local current = redis.call('GET', KEYS[1])
		if not current then
			redis.call('SET', KEYS[1], 1, 'EX', ARGV[1])
			return 1
		end
		local count = redis.call('INCR', KEYS[1])
		if redis.call('TTL', KEYS[1]) < 0 then
			redis.call('EXPIRE', KEYS[1], ARGV[1])
		end
		return count
	`
	return client.Eval(ctx, script, []string{key}, rateLimitWindowSeconds).Int64()
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
