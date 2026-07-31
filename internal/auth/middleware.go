package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	ClaimsKey contextKey = "claims"
)

type Claims struct {
	TeamID      string
	UserID      string
	Role        string
	APIKeyID    string
	Permissions []string
	Plan        string
}

type TeamContext struct {
	TeamID string
	Role   string
	Plan   string
}

type APIKeyContext struct {
	TeamID      string
	APIKeyID    string
	Permissions []string
	Plan        string
}

type SupabaseClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

func GetClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(ClaimsKey).(*Claims)
	return claims
}

func GetTeamIDFromContext(ctx context.Context) string {
	if claims := GetClaimsFromContext(ctx); claims != nil {
		return claims.TeamID
	}
	return ""
}

func GetUserIDFromContext(ctx context.Context) string {
	if claims := GetClaimsFromContext(ctx); claims != nil {
		return claims.UserID
	}
	return ""
}

var jwtKeyfunc keyfunc.Keyfunc
var jwtIssuer string

func InitJWT(supabaseURL string) error {
	supabaseURL = strings.TrimRight(supabaseURL, "/")
	jwksURL := supabaseURL + "/auth/v1/.well-known/jwks.json"
	jwtKeyfunc = nil
	jwtIssuer = supabaseURL + "/auth/v1"
	var err error
	jwtKeyfunc, err = keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return fmt.Errorf("failed to init JWKS: %w", err)
	}
	return nil
}

func VerifySupabaseJWT(tokenString string) (*SupabaseClaims, error) {
	if jwtKeyfunc == nil {
		return nil, fmt.Errorf("jwt verification is not configured")
	}
	claims := &SupabaseClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		jwtKeyfunc.KeyfuncCtx(context.Background()),
		jwt.WithIssuer(jwtIssuer),
		jwt.WithAudience("authenticated"),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.Sub == "" {
		return nil, errors.New("token subject is missing")
	}
	return claims, nil
}

var verifyAPIKeyFunc func(rawKey string) (string, string, error)

var verifyAPIKeyContextFunc func(ctx context.Context, rawKey string) (*APIKeyContext, error)

var resolveUserTeamFunc func(ctx context.Context, userID, requestedTeamID string) (*TeamContext, error)

func SetVerifyAPIKeyFunc(fn func(string) (string, string, error)) {
	verifyAPIKeyFunc = fn
}

func SetVerifyAPIKeyContextFunc(fn func(context.Context, string) (*APIKeyContext, error)) {
	verifyAPIKeyContextFunc = fn
}

func SetUserTeamResolver(fn func(context.Context, string, string) (*TeamContext, error)) {
	resolveUserTeamFunc = fn
}

func VerifyAPIKey(rawKey string) (string, string, error) {
	if verifyAPIKeyFunc == nil {
		return "", "", fmt.Errorf("api key verification not configured")
	}
	return verifyAPIKeyFunc(rawKey)
}

func VerifyAPIKeyContext(ctx context.Context, rawKey string) (*APIKeyContext, error) {
	if verifyAPIKeyContextFunc != nil {
		return verifyAPIKeyContextFunc(ctx, rawKey)
	}
	if verifyAPIKeyFunc == nil {
		return nil, fmt.Errorf("api key verification not configured")
	}
	teamID, apiKeyID, err := verifyAPIKeyFunc(rawKey)
	if err != nil {
		return nil, err
	}
	return &APIKeyContext{
		TeamID:      teamID,
		APIKeyID:    apiKeyID,
		Permissions: []string{"send"},
	}, nil
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSONError(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			writeJSONError(w, "invalid authorization header", http.StatusUnauthorized)
			return
		}

		token := parts[1]

		if strings.HasPrefix(token, "re_") {
			apiKey, err := VerifyAPIKeyContext(r.Context(), token)
			if err != nil || apiKey == nil || apiKey.TeamID == "" || apiKey.APIKeyID == "" {
				writeJSONError(w, "invalid api key", http.StatusUnauthorized)
				return
			}
			claims := &Claims{
				TeamID:      apiKey.TeamID,
				Role:        "api_key",
				APIKeyID:    apiKey.APIKeyID,
				Permissions: apiKey.Permissions,
				Plan:        apiKey.Plan,
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if strings.HasPrefix(token, "ey") {
			supClaims, err := VerifySupabaseJWT(token)
			if err != nil {
				writeJSONError(w, "invalid jwt", http.StatusUnauthorized)
				return
			}
			claims := &Claims{
				UserID: supClaims.Sub,
				Role:   "user",
			}
			requestedTeamID := r.Header.Get("X-Team-ID")
			if requestedTeamID != "" {
				if resolveUserTeamFunc == nil {
					writeJSONError(w, "team resolver is not configured", http.StatusServiceUnavailable)
					return
				}
				team, err := resolveUserTeamFunc(r.Context(), supClaims.Sub, requestedTeamID)
				if err != nil {
					writeJSONError(w, "team access denied", http.StatusForbidden)
					return
				}
				claims.TeamID = team.TeamID
				claims.Role = team.Role
				claims.Plan = team.Plan
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		writeJSONError(w, "invalid token format", http.StatusUnauthorized)
	})
}

func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaimsFromContext(r.Context())
			if claims == nil {
				writeJSONError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			allowed := false
			for _, role := range roles {
				if claims.Role == role {
					allowed = true
					break
				}
			}
			if !allowed {
				writeJSONError(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RateLimitKeyFromContext(ctx context.Context) string {
	claims := GetClaimsFromContext(ctx)
	if claims == nil {
		return "anonymous"
	}
	if claims.TeamID != "" {
		return "team:" + claims.TeamID
	}
	return "user:" + claims.UserID
}

func HasAnyRole(claims *Claims, roles ...string) bool {
	if claims == nil {
		return false
	}
	for _, role := range roles {
		if claims.Role == role {
			return true
		}
	}
	return false
}

func HasPermission(claims *Claims, permission string) bool {
	if claims == nil {
		return false
	}
	if claims.Role != "api_key" {
		return true
	}
	for _, current := range claims.Permissions {
		if current == permission || current == "*" {
			return true
		}
	}
	return false
}

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
