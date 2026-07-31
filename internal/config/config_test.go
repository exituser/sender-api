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

func TestValidateProductionAllowsOptionalSNSIntegrations(t *testing.T) {
	cfg := &Config{
		Env:         "production",
		CORSOrigins: "https://app.example.com",
		DatabaseURL: "postgresql://db/sender_api",
		RedisURL:    "redis:6379",
		SupabaseURL: "https://project.supabase.co",
		AWSRegion:   "eu-west-1",
		Debug:       false,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected production config without optional callbacks to validate: %v", err)
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
