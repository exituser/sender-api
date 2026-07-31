package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/pkg/validator"
)

type WebhookPayload struct {
	ID        string          `json:"id"`
	Event     string          `json:"event"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

func SignPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifySignature(payload []byte, signature string, secret string) bool {
	expected := SignPayload(payload, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func SendWebhook(url string, secret string, event string, payload any) error {
	if !validator.IsValidURL(url) {
		return fmt.Errorf("unsafe webhook url")
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	webhookPayload := WebhookPayload{
		ID:        uuid.New().String(),
		Event:     event,
		Payload:   payloadBytes,
		Timestamp: time.Now().UTC(),
	}

	body, err := json.Marshal(webhookPayload)
	if err != nil {
		return fmt.Errorf("marshal webhook body: %w", err)
	}

	signature := SignPayload(body, secret)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: safeDialContext,
		},
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Signature", signature)
		req.Header.Set("X-Webhook-Event", event)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send webhook: %w", err)
		} else {
			responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
			resp.Body.Close()
			if resp.StatusCode < 400 {
				return nil
			}
			lastErr = fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(responseBody))
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
				return lastErr
			}
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 250 * time.Millisecond)
		}
	}
	return lastErr
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid webhook address: %w", err)
	}
	ipAddresses, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("resolve webhook host: %w", err)
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	var lastErr error
	for _, ip := range ipAddresses {
		if validator.IsPrivateIP(ip) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("webhook host resolves only to private addresses")
}
