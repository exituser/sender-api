package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sender-api/sender-api/internal/auth"
)

func RateLimit(redisClient *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redisClient == nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			key := getRateLimitKey(ctx)
			limit := getLimitForPlan(ctx)

			count, err := redisClient.Incr(ctx, key).Result()
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			if count == 1 {
				redisClient.Expire(ctx, key, time.Second)
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, limit-int(count))))

			if int(count) > limit {
				w.Header().Set("Retry-After", "1")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"error":"rate limit exceeded"}`)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func getRateLimitKey(ctx context.Context) string {
	claims := auth.GetClaimsFromContext(ctx)
	if claims == nil {
		return "ratelimit:anonymous"
	}
	if claims.TeamID != "" {
		return "ratelimit:team:" + claims.TeamID
	}
	if claims.UserID != "" {
		return "ratelimit:user:" + claims.UserID
	}
	return "ratelimit:anonymous"
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
