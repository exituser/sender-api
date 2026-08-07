package config

import (
	"crypto/tls"
	"testing"
)

func TestParseRedisOptionsSupportsHostPort(t *testing.T) {
	options, err := ParseRedisOptions("localhost:6379")
	if err != nil {
		t.Fatalf("parse raw redis address: %v", err)
	}
	if options.Addr != "localhost:6379" || options.TLSConfig != nil {
		t.Fatalf("unexpected raw redis options: addr=%q tls=%t", options.Addr, options.TLSConfig != nil)
	}
}

func TestParseRedisOptionsSupportsTLSURL(t *testing.T) {
	options, err := ParseRedisOptions("rediss://:secret@redis.example.com:6380/2")
	if err != nil {
		t.Fatalf("parse TLS redis URL: %v", err)
	}
	if options.Addr != "redis.example.com:6380" || options.DB != 2 || options.Password == "" || options.TLSConfig == nil {
		t.Fatalf("unexpected TLS redis options: addr=%q db=%d password_set=%t tls=%t", options.Addr, options.DB, options.Password != "", options.TLSConfig != nil)
	}
	if options.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", options.TLSConfig.MinVersion)
	}
}

func TestParseRedisOptionsDefaultsURLPort(t *testing.T) {
	options, err := ParseRedisOptions("rediss://redis.example.com/0")
	if err != nil {
		t.Fatalf("parse Redis URL without port: %v", err)
	}
	if options.Addr != "redis.example.com:6379" {
		t.Fatalf("expected default Redis port, got %q", options.Addr)
	}
}

func TestParseRedisOptionsRejectsQueryParameters(t *testing.T) {
	if _, err := ParseRedisOptions("rediss://redis.example.com:6380/0?insecure=true"); err == nil {
		t.Fatal("expected query parameters to be rejected")
	}
}
