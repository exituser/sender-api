package config

import (
	"fmt"
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

	DatabaseURL string
	RedisURL    string

	AWSRegion           string
	AWSAccessKeyID      string
	AWSSecretAccessKey  string
	AWSESConfigSet      string
	OutboundSESTopicArn string

	SupabaseURL        string
	SupabaseAnonKey    string
	SupabaseServiceKey string

	SentryDSN string

	InboundS3Bucket     string
	InboundSQSQueueURL  string
	InboundSNSTopicArn  string
	InboundWebhookToken string
}

func Load() *Config {
	godotenv.Load()

	return &Config{
		Port:        getEnv("PORT", "8080"),
		Env:         getEnv("ENV", "development"),
		Debug:       getBoolEnv("DEBUG", true),
		CORSOrigins: getEnv("CORS_ORIGINS", "http://localhost:3000"),

		DatabaseURL: getEnv("DATABASE_URL", "postgresql://supabase_admin:postgres@localhost:5432/sender_api"),
		RedisURL:    getEnv("REDIS_URL", "localhost:6379"),

		AWSRegion:           getEnv("AWS_REGION", "eu-west-1"),
		AWSAccessKeyID:      os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey:  os.Getenv("AWS_SECRET_ACCESS_KEY"),
		AWSESConfigSet:      os.Getenv("AWS_SES_CONFIGSET"),
		OutboundSESTopicArn: os.Getenv("OUTBOUND_SES_TOPIC_ARN"),

		SupabaseURL:        getEnv("SUPABASE_URL", "http://localhost:54321"),
		SupabaseAnonKey:    os.Getenv("SUPABASE_ANON_KEY"),
		SupabaseServiceKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),

		SentryDSN: os.Getenv("SENTRY_DSN"),

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
	if err := validateCORSOrigins(c.CORSOrigins); err != nil {
		return err
	}
	if c.IsProduction() {
		if c.Debug {
			return fmt.Errorf("DEBUG must be false in production")
		}
		for key, value := range map[string]string{
			"DATABASE_URL": c.DatabaseURL,
			"REDIS_URL":    c.RedisURL,
			"SUPABASE_URL": c.SupabaseURL,
			"AWS_REGION":   c.AWSRegion,
			"CORS_ORIGINS": c.CORSOrigins,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s is required in production", key)
			}
		}
		if strings.TrimSpace(c.InboundSNSTopicArn) == "" {
			return fmt.Errorf("INBOUND_SNS_TOPIC_ARN is required in production")
		}
		if strings.TrimSpace(c.OutboundSESTopicArn) == "" {
			return fmt.Errorf("OUTBOUND_SES_TOPIC_ARN is required in production")
		}
	}
	if c.InboundSQSQueueURL != "" && strings.TrimSpace(c.InboundS3Bucket) == "" {
		return fmt.Errorf("INBOUND_S3_BUCKET is required when INBOUND_SQS_QUEUE_URL is configured")
	}
	if c.InboundSQSQueueURL != "" && strings.TrimSpace(c.InboundSNSTopicArn) == "" {
		return fmt.Errorf("INBOUND_SNS_TOPIC_ARN is required when INBOUND_SQS_QUEUE_URL is configured")
	}
	return nil
}

func validateCORSOrigins(raw string) error {
	origins := strings.Split(raw, ",")
	if len(origins) == 0 {
		return fmt.Errorf("CORS_ORIGINS must contain an explicit origin allowlist")
	}
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		parsed, err := url.Parse(origin)
		if err != nil || origin == "" || strings.Contains(origin, "*") ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(parsed.Path != "" && parsed.Path != "/") {
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
