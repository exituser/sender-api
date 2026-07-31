package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port    string
	Env     string
	Debug   bool
	CORSOrigins string

	DatabaseURL         string
	RedisURL            string

	AWSRegion           string
	AWSAccessKeyID      string
	AWSSecretAccessKey  string
	AWSESConfigSet      string

	SupabaseURL        string
	SupabaseAnonKey    string
	SupabaseServiceKey string

	SentryDSN string

	InboundS3Bucket    string
	InboundSQSQueueURL string
	InboundWebhookToken string
}

func Load() *Config {
	godotenv.Load()

	return &Config{
		Port:    getEnv("PORT", "8080"),
		Env:     getEnv("ENV", "development"),
		Debug:   getBoolEnv("DEBUG", true),
		CORSOrigins: getEnv("CORS_ORIGINS", "http://localhost:3000"),

		DatabaseURL:         getEnv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/sender_api"),
		RedisURL:            getEnv("REDIS_URL", "localhost:6379"),

		AWSRegion:           getEnv("AWS_REGION", "eu-west-1"),
		AWSAccessKeyID:      os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey:  os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AWSESConfigSet:      os.Getenv("AWS_SES_CONFIGSET"),

		SupabaseURL:        getEnv("SUPABASE_URL", "http://localhost:54321"),
		SupabaseAnonKey:    os.Getenv("SUPABASE_ANON_KEY"),
		SupabaseServiceKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),

		SentryDSN: os.Getenv("SENTRY_DSN"),

		InboundS3Bucket:    getEnv("INBOUND_S3_BUCKET", "sender-api-inbound"),
		InboundSQSQueueURL: os.Getenv("INBOUND_SQS_QUEUE_URL"),
		InboundWebhookToken: os.Getenv("INBOUND_WEBHOOK_TOKEN"),
	}
}

func (c *Config) IsProduction() bool {
	return c.Env == "production"
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
