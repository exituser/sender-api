package config

import "testing"

func TestValidateRejectsWildcardCORS(t *testing.T) {
	cfg := &Config{CORSOrigins: "*"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected wildcard CORS to be rejected")
	}
}

func TestValidateRequiresInboundBucketWithQueue(t *testing.T) {
	cfg := &Config{CORSOrigins: "http://localhost:3000", InboundSQSQueueURL: "https://sqs.example/queue"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected inbound bucket requirement")
	}
}

func TestValidateRejectsMalformedCORSOrigin(t *testing.T) {
	cfg := &Config{CORSOrigins: "https://example.com/path"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected malformed CORS origin to be rejected")
	}
}

func TestValidateRequiresInboundSNSTopicWithQueue(t *testing.T) {
	cfg := &Config{
		CORSOrigins:        "http://localhost:3000",
		InboundS3Bucket:    "sender-api-inbound",
		InboundSQSQueueURL: "https://sqs.example/queue",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected inbound SNS topic requirement")
	}
}

func TestValidateProductionRequiresCoreOperationalSettings(t *testing.T) {
	cfg := &Config{
		Env:                 "production",
		CORSOrigins:         "https://app.example.com",
		DatabaseURL:         "postgresql://db/sender_api",
		RedisURL:            "rediss://redis.example:6380",
		SupabaseURL:         "https://project.supabase.co",
		AWSRegion:           "eu-west-1",
		MetricsToken:        "metrics-secret",
		DailyRecipientLimit: 1000,
		Debug:               false,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected core production config to validate: %v", err)
	}
}

func TestValidateProductionAllowsComposeServices(t *testing.T) {
	cfg := &Config{
		Env:                 "production",
		CORSOrigins:         "https://app.example.com",
		DatabaseURL:         "postgresql://supabase_admin:secret@db:5432/sender_api",
		RedisURL:            "redis:6379",
		SupabaseURL:         "https://project.supabase.co",
		AWSRegion:           "eu-west-1",
		MetricsToken:        "metrics-secret",
		DailyRecipientLimit: 1000,
		Debug:               false,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected Compose production config to validate: %v", err)
	}
}

func TestValidateRequiresSESConfigSetForOutboundEvents(t *testing.T) {
	cfg := &Config{
		CORSOrigins:         "http://localhost:3000",
		OutboundSESTopicArn: "arn:aws:sns:eu-west-1:123:events",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected configuration set requirement for outbound events")
	}
}

func TestValidateCanRequireOutboundSESEvents(t *testing.T) {
	cfg := &Config{CORSOrigins: "http://localhost:3000", RequireOutboundSESEvents: true}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected outbound SES event topic requirement")
	}
}

func TestValidateProductionRejectsHTTPOrigin(t *testing.T) {
	cfg := &Config{Env: "production", CORSOrigins: "http://app.example.com"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production HTTP origin to be rejected")
	}
}

func TestValidateProductionRejectsHTTPAuthEndpoint(t *testing.T) {
	cfg := &Config{
		Env:          "production",
		CORSOrigins:  "https://app.example.com",
		SupabaseURL:  "http://auth.internal",
		DatabaseURL:  "postgresql://db/sender_api",
		RedisURL:     "rediss://redis.example:6380",
		AWSRegion:    "eu-west-1",
		MetricsToken: "metrics-secret",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production HTTP auth endpoint to be rejected")
	}
}

func TestValidateRejectsInvalidLowVolumeSettings(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "database pool", cfg: Config{CORSOrigins: "http://localhost:3000", DBMaxConns: -1}},
		{name: "redis pool", cfg: Config{CORSOrigins: "http://localhost:3000", RedisPoolSize: -1}},
		{name: "trace sample rate", cfg: Config{CORSOrigins: "http://localhost:3000", SentryTraceSampleRate: 1.1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err == nil {
				t.Fatal("expected invalid low-volume setting to be rejected")
			}
		})
	}
}

func TestValidateProductionRejectsHTTPStripeReturnURL(t *testing.T) {
	cfg := &Config{
		Env:                 "production",
		CORSOrigins:         "https://app.example.com",
		DatabaseURL:         "postgresql://db/sender_api",
		RedisURL:            "rediss://redis.example:6380",
		SupabaseURL:         "https://project.supabase.co",
		AWSRegion:           "eu-west-1",
		MetricsToken:        "metrics-secret",
		DailyRecipientLimit: 1000,
		Debug:               false,
		StripeSecretKey:     "sk_live_test",
		StripeWebhookSecret: "whsec_test",
		StripePricePro:      "price_pro",
		StripePriceScale:    "price_scale",
		StripeSuccessURL:    "https://app.example.com/success",
		StripeCancelURL:     "https://app.example.com/cancel",
		StripeReturnURL:     "http://app.example.com/billing",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production HTTP Stripe return URL to be rejected")
	}
}

func TestValidateProductionRejectsRemoteDatabaseWithoutTLS(t *testing.T) {
	cfg := &Config{
		Env:                 "production",
		CORSOrigins:         "https://app.example.com",
		DatabaseURL:         "postgresql://db.example.com/sender_api",
		RedisURL:            "rediss://redis.example.com:6380",
		SupabaseURL:         "https://project.supabase.co",
		AWSRegion:           "eu-west-1",
		MetricsToken:        "metrics-secret",
		DailyRecipientLimit: 1000,
		Debug:               false,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected remote production database without TLS to be rejected")
	}
}

func TestValidateProductionRejectsRemoteRedisWithoutTLS(t *testing.T) {
	cfg := &Config{
		Env:                 "production",
		CORSOrigins:         "https://app.example.com",
		DatabaseURL:         "postgresql://db.example.com/sender_api?sslmode=require",
		RedisURL:            "redis://redis.example.com:6379",
		SupabaseURL:         "https://project.supabase.co",
		AWSRegion:           "eu-west-1",
		MetricsToken:        "metrics-secret",
		DailyRecipientLimit: 1000,
		Debug:               false,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected remote production Redis without TLS to be rejected")
	}
}

func TestValidateRejectsInvalidInboundVisibilityTimeout(t *testing.T) {
	cfg := &Config{CORSOrigins: "http://localhost:3000", InboundVisibilityTimeoutSeconds: 43201}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid inbound visibility timeout to be rejected")
	}
}

func TestValidateRequiresUnsubscribeSettingsTogether(t *testing.T) {
	cfg := &Config{CORSOrigins: "http://localhost:3000", PublicAPIURL: "http://localhost:8080"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsubscribe settings to be configured together")
	}
}

func TestValidateAcceptsUnsubscribeSettings(t *testing.T) {
	cfg := &Config{
		CORSOrigins:              "http://localhost:3000",
		PublicAPIURL:             "http://localhost:8080",
		UnsubscribeSigningSecret: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected unsubscribe settings to validate: %v", err)
	}
}
