package config

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// ParseRedisOptions accepts the legacy host:port form and redis:// or
// rediss:// URLs without ever placing credentials in the returned error.
func ParseRedisOptions(raw string) (*redis.Options, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("REDIS_URL is empty")
	}
	if !strings.Contains(value, "://") {
		return &redis.Options{Addr: value}, nil
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "redis" && parsed.Scheme != "rediss") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("REDIS_URL must be a valid Redis URL")
	}

	database := 0
	path := strings.TrimPrefix(parsed.Path, "/")
	if path != "" {
		database, err = strconv.Atoi(path)
		if err != nil || database < 0 {
			return nil, fmt.Errorf("REDIS_URL database must be a non-negative integer")
		}
	}

	options := &redis.Options{
		Addr:     parsed.Host,
		DB:       database,
		Username: "",
	}
	if parsed.Port() == "" {
		options.Addr = net.JoinHostPort(parsed.Hostname(), "6379")
	}
	if parsed.User != nil {
		options.Username = parsed.User.Username()
		if password, ok := parsed.User.Password(); ok {
			options.Password = password
		}
	}
	if parsed.Scheme == "rediss" {
		options.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: parsed.Hostname(),
		}
	}
	return options, nil
}
