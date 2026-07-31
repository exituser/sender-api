package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	Env         string
	Debug       bool
	CORSOrigins string

	DatabaseURL        string
	RedisURL           string
	DBMaxConns         int
	DBMinConns         int
	RedisPoolSize      int
	WorkerPollInterval time.Duration

	AWSRegion           string
	AWSAccessKeyID      string
	AWSSecretAccessKey  string
	AWSESConfigSet      string
	OutboundSESTopicArn string

	SupabaseURL        string
	SupabaseAnonKey    string
	SupabaseServiceKey string

	SentryDSN             string
	SentryTraceSampleRate float64
	MetricsToken          string
	DailyRecipientLimit   int

	InboundS3Bucket     string
	InboundSQSQueueURL  string
	InboundSNSTopicArn  string
	InboundWebhookToken string
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		Port:        getEnv("PORT", "8080"),
		Env:         getEnv("ENV", "development"),
		Debug:       getBoolEnv("DEBUG", true),
		CORSOrigins: getEnv("CORS_ORIGINS", "http://localhost:3000"),

		DatabaseURL:        getEnv("DATABASE_URL", "postgresql://supabase_admin:postgres@localhost:5432/sender_api"),
		RedisURL:           getEnv("REDIS_URL", "localhost:6379"),
		DBMaxConns:         getPositiveIntEnv("DB_MAX_CONNS", 4),
		DBMinConns:         getNonNegativeIntEnv("DB_MIN_CONNS", 0),
		RedisPoolSize:      getPositiveIntEnv("REDIS_POOL_SIZE", 4),
		WorkerPollInterval: getDurationEnv("WORKER_POLL_INTERVAL", 5*time.Second),

		AWSRegion:           getEnv("AWS_REGION", "eu-west-1"),
		AWSAccessKeyID:      os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey:  os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AWSESConfigSet:      os.Getenv("AWS_SES_CONFIGSET"),
		OutboundSESTopicArn: os.Getenv("OUTBOUND_SES_TOPIC_ARN"),

		SupabaseURL:        getEnv("SUPABASE_URL", "http://localhost:54321"),
		SupabaseAnonKey:    os.Getenv("SUPABASE_ANON_KEY"),
		SupabaseServiceKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),

		SentryDSN:             os.Getenv("SENTRY_DSN"),
		SentryTraceSampleRate: getFloatEnv("SENTRY_TRACES_SAMPLE_RATE", 0),
		MetricsToken:          os.Getenv("METRICS_TOKEN"),
		DailyRecipientLimit:   getPositiveIntEnv("DAILY_RECIPIENT_LIMIT", 1000),

		InboundS3Bucket:     getEnv("INBOUND_S3_BUCKET", "sender-api-inbound"),
		InboundSQSQueueURL:  os.Getenv("INBOUND_SQS_QUEUE_URL"),
		InboundSNSTopicArn:  os.Getenv("INBOUND_SNS_TOPIC_ARN"),
		InboundWebhookToken: os.Getenv("INBOUND_WEBHOOK_TOKEN"),
	}
}

func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

func (c *Config) Validate() error {
	if err := validateCORSOrigins(c.CORSOrigins, c.IsProduction()); err != nil {
		return err
	}
	if c.DBMaxConns != 0 && c.DBMaxConns < 1 {
		return fmt.Errorf("DB_MAX_CONNS must be at least 1")
	}
	if c.DBMinConns < 0 || (c.DBMaxConns != 0 && c.DBMinConns > c.DBMaxConns) {
		return fmt.Errorf("DB_MIN_CONNS must be between 0 and DB_MAX_CONNS")
	}
	if c.RedisPoolSize != 0 && c.RedisPoolSize < 1 {
		return fmt.Errorf("REDIS_POOL_SIZE must be at least 1")
	}
	if c.WorkerPollInterval < 0 {
		return fmt.Errorf("WORKER_POLL_INTERVAL must not be negative")
	}
	if c.DailyRecipientLimit < 0 {
		return fmt.Errorf("DAILY_RECIPIENT_LIMIT must not be negative")
	}
	if math.IsNaN(c.SentryTraceSampleRate) || c.SentryTraceSampleRate < 0 || c.SentryTraceSampleRate > 1 {
		return fmt.Errorf("SENTRY_TRACES_SAMPLE_RATE must be between 0 and 1")
	}
	if c.IsProduction() {
		if c.Debug {
			return fmt.Errorf("DEBUG must be false in production")
		}
		supabaseURL, err := url.Parse(c.SupabaseURL)
		if err != nil || supabaseURL.Scheme != "https" || supabaseURL.Hostname() == "" {
			return fmt.Errorf("SUPABASE_URL must be an HTTPS URL in production")
		}
		if c.DatabaseURL == "postgresql://supabase_admin:postgres@localhost:5432/sender_api" ||
			c.SupabaseURL == "http://localhost:54321" || c.RedisURL == "localhost:6379" || c.RedisURL == "redis:6379" {
			return fmt.Errorf("development service defaults are not allowed in production")
		}
		for key, value := range map[string]string{
			"DATABASE_URL":  c.DatabaseURL,
			"REDIS_URL":     c.RedisURL,
			"SUPABASE_URL":  c.SupabaseURL,
			"AWS_REGION":    c.AWSRegion,
			"CORS_ORIGINS":  c.CORSOrigins,
			"METRICS_TOKEN": c.MetricsToken,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s is required in production", key)
			}
		}
		if c.DailyRecipientLimit < 1 {
			return fmt.Errorf("DAILY_RECIPIENT_LIMIT must be positive in production")
		}
	}
	if c.OutboundSESTopicArn != "" && strings.TrimSpace(c.AWSESConfigSet) == "" {
		return fmt.Errorf("AWS_SES_CONFIGSET is required when OUTBOUND_SES_TOPIC_ARN is configured")
	}
	if c.InboundSQSQueueURL != "" && strings.TrimSpace(c.InboundS3Bucket) == "" {
		return fmt.Errorf("INBOUND_S3_BUCKET is required when INBOUND_SQS_QUEUE_URL is configured")
	}
	if c.InboundSQSQueueURL != "" && strings.TrimSpace(c.InboundSNSTopicArn) == "" {
		return fmt.Errorf("INBOUND_SNS_TOPIC_ARN is required when INBOUND_SQS_QUEUE_URL is configured")
	}
	return nil
}

func validateCORSOrigins(raw string, requireHTTPS bool) error {
	origins := strings.Split(raw, ",")
	if len(origins) == 0 {
		return fmt.Errorf("CORS_ORIGINS must contain an explicit origin allowlist")
	}
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		parsed, err := url.Parse(origin)
		if err != nil || origin == "" || strings.Contains(origin, "*") ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") ||
			(requireHTTPS && parsed.Scheme != "https") || parsed.Hostname() == "" ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(parsed.Path != "" && parsed.Path != "/") ||
			(requireHTTPS && (parsed.Hostname() == "localhost" || strings.HasSuffix(parsed.Hostname(), ".localhost"))) {
			return fmt.Errorf("CORS_ORIGINS must contain explicit http(s) origins")
		}
	}
	return nil
}

func (c *Config) RateLimitPerSecond() int {
	switch c.Env {
	case "production":
		return 50
	default:
		return 10
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getPositiveIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	value, err := strconv.Atoi(v)
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func getNonNegativeIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	value, err := strconv.Atoi(v)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func getFloatEnv(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(value) || value < 0 || value > 1 {
		return fallback
	}
	return value
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
