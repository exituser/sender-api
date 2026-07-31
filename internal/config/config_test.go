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

func TestValidateProductionRequiresExactSNSTopics(t *testing.T) {
	cfg := &Config{
		Env:         "production",
		CORSOrigins: "https://app.example.com",
		DatabaseURL: "postgresql://db/sender_api",
		RedisURL:    "redis:6379",
		SupabaseURL: "https://project.supabase.co",
		AWSRegion:   "eu-west-1",
		Debug:       false,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production SNS topic requirement")
	}
	cfg.InboundSNSTopicArn = "arn:aws:sns:eu-west-1:123:inbound"
	cfg.OutboundSESTopicArn = "arn:aws:sns:eu-west-1:123:outbound"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected complete production config to validate: %v", err)
	}
}
