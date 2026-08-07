package sns

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- SNS SignatureVersion 1 requires SHA-1.
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Notification struct {
	Type             string
	Message          string
	MessageID        string
	Subject          string
	Timestamp        string
	TopicArn         string
	SigningCertURL   string
	Signature        string
	SignatureVersion string
	SubscribeURL     string
	UnsubscribeURL   string
	Token            string
}

type certificateCacheEntry struct {
	certificate *x509.Certificate
	expiresAt   time.Time
}

var certificateCache = struct {
	sync.Mutex
	entries map[string]certificateCacheEntry
}{entries: make(map[string]certificateCacheEntry)}

var (
	ErrStaleNotification   = errors.New("stale SNS notification")
	ErrInvalidNotification = errors.New("invalid SNS notification")
)

var snsCertificateHost = regexp.MustCompile(`^sns\.[a-z0-9-]+\.amazonaws\.com(\.cn)?$`)

func VerifyNotification(ctx context.Context, notification Notification, region string, now time.Time) error {
	return verifyNotification(ctx, notification, region, now, false)
}

// VerifyNotificationForQueue accepts signed notifications delayed by the
// asynchronous SNS-to-SQS transport. Side effects are deduplicated by the
// provider/SNS message identifiers downstream.
func VerifyNotificationForQueue(ctx context.Context, notification Notification, region string, now time.Time) error {
	return verifyNotification(ctx, notification, region, now, true)
}

func verifyNotification(ctx context.Context, notification Notification, region string, now time.Time, allowDelayed bool) error {
	if notification.Type != "Notification" && notification.Type != "SubscriptionConfirmation" && notification.Type != "UnsubscribeConfirmation" {
		return fmt.Errorf("%w: unsupported SNS message type", ErrInvalidNotification)
	}
	if notification.Message == "" || notification.MessageID == "" || notification.TopicArn == "" ||
		notification.Timestamp == "" || notification.Signature == "" || notification.SigningCertURL == "" ||
		(notification.SignatureVersion != "1" && notification.SignatureVersion != "2") {
		return fmt.Errorf("%w: incomplete SNS notification", ErrInvalidNotification)
	}
	if notification.Type == "SubscriptionConfirmation" && (notification.SubscribeURL == "" || notification.Token == "") {
		return fmt.Errorf("%w: incomplete SNS subscription confirmation", ErrInvalidNotification)
	}
	if notification.Type == "UnsubscribeConfirmation" && (notification.UnsubscribeURL == "" || notification.Token == "") {
		return fmt.Errorf("%w: incomplete SNS unsubscription confirmation", ErrInvalidNotification)
	}
	timestamp, err := time.Parse(time.RFC3339, notification.Timestamp)
	if err != nil || timestamp.After(now.Add(5*time.Minute)) || (!allowDelayed && timestamp.Before(now.Add(-5*time.Minute))) {
		return fmt.Errorf("%w", ErrStaleNotification)
	}
	if !validCertificateURL(notification.SigningCertURL, region) {
		return fmt.Errorf("%w: invalid SNS signing certificate URL", ErrInvalidNotification)
	}
	certificate, err := getCertificate(ctx, notification.SigningCertURL)
	if err != nil {
		return fmt.Errorf("load SNS signing certificate: %w", err)
	}
	publicKey, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("%w: SNS signing certificate is not RSA", ErrInvalidNotification)
	}
	signature, err := base64.StdEncoding.DecodeString(notification.Signature)
	if err != nil {
		return fmt.Errorf("%w: decode SNS signature: %v", ErrInvalidNotification, err)
	}
	message := []byte(StringToSign(notification))
	switch notification.SignatureVersion {
	case "1":
		digest := sha1.Sum(message) // #nosec G401 -- required by SNS SignatureVersion 1.
		if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA1, digest[:], signature); err != nil {
			return fmt.Errorf("%w: verify SNS signature: %v", ErrInvalidNotification, err)
		}
		return nil
	case "2":
		digest := sha256.Sum256(message)
		if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
			return fmt.Errorf("%w: verify SNS signature: %v", ErrInvalidNotification, err)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported SNS signature version", ErrInvalidNotification)
	}
}

func StringToSign(notification Notification) string {
	var builder strings.Builder
	write := func(key, value string) {
		builder.WriteString(key)
		builder.WriteByte('\n')
		builder.WriteString(value)
		builder.WriteByte('\n')
	}
	write("Message", notification.Message)
	write("MessageId", notification.MessageID)
	if notification.Type == "Notification" && notification.Subject != "" {
		write("Subject", notification.Subject)
	}
	if notification.Type == "SubscriptionConfirmation" {
		write("SubscribeURL", notification.SubscribeURL)
	}
	write("Timestamp", notification.Timestamp)
	if notification.Type == "UnsubscribeConfirmation" {
		write("UnsubscribeURL", notification.UnsubscribeURL)
	}
	if notification.Type != "Notification" {
		write("Token", notification.Token)
	}
	write("TopicArn", notification.TopicArn)
	write("Type", notification.Type)
	return builder.String()
}

func ConfirmSubscription(ctx context.Context, notification Notification, region string) error {
	if notification.Type != "SubscriptionConfirmation" {
		return fmt.Errorf("%w: unsupported SNS confirmation type", ErrInvalidNotification)
	}
	parsed, err := url.Parse(notification.SubscribeURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: invalid SNS subscribe URL", ErrInvalidNotification)
	}
	host := strings.ToLower(parsed.Hostname())
	if !snsCertificateHost.MatchString(host) {
		return fmt.Errorf("%w: invalid SNS subscribe host", ErrInvalidNotification)
	}
	if region != "" && host != "sns."+strings.ToLower(region)+".amazonaws.com" && host != "sns."+strings.ToLower(region)+".amazonaws.com.cn" {
		return fmt.Errorf("%w: invalid SNS subscribe host", ErrInvalidNotification)
	}
	query := parsed.Query()
	if query.Get("Action") != "ConfirmSubscription" || query.Get("Token") == "" || query.Get("TopicArn") == "" ||
		(notification.Token != "" && query.Get("Token") != notification.Token) ||
		(notification.TopicArn != "" && query.Get("TopicArn") != notification.TopicArn) {
		return fmt.Errorf("%w: incomplete SNS subscribe URL", ErrInvalidNotification)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}).Do(request)
	if err != nil {
		return fmt.Errorf("confirm SNS subscription: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8<<10))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("confirm SNS subscription: provider returned %d", response.StatusCode)
	}
	return nil
}

func validCertificateURL(rawURL, region string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if !snsCertificateHost.MatchString(host) || !strings.HasSuffix(strings.ToLower(parsed.Path), ".pem") {
		return false
	}
	if region != "" {
		return host == "sns."+strings.ToLower(region)+".amazonaws.com" || host == "sns."+strings.ToLower(region)+".amazonaws.com.cn"
	}
	return true
}

func getCertificate(ctx context.Context, rawURL string) (*x509.Certificate, error) {
	now := time.Now()
	certificateCache.Lock()
	if entry, ok := certificateCache.entries[rawURL]; ok && entry.expiresAt.After(now) {
		certificateCache.Unlock()
		return entry.certificate, nil
	}
	certificateCache.Unlock()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := (&http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}).Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("certificate endpoint returned %d", response.StatusCode)
	}
	pemBytes, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	certificate, err := parseCertificatePEM(pemBytes)
	if err != nil {
		return nil, err
	}

	certificateCache.Lock()
	certificateCache.entries[rawURL] = certificateCacheEntry{certificate: certificate, expiresAt: now.Add(10 * time.Minute)}
	certificateCache.Unlock()
	return certificate, nil
}

func parseCertificatePEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}
