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
}

type certificateCacheEntry struct {
	certificate *x509.Certificate
	expiresAt   time.Time
}

var certificateCache = struct {
	sync.Mutex
	entries map[string]certificateCacheEntry
}{entries: make(map[string]certificateCacheEntry)}

var snsCertificateHost = regexp.MustCompile(`^sns\.[a-z0-9-]+\.amazonaws\.com(\.cn)?$`)

func VerifyNotification(ctx context.Context, notification Notification, region string, now time.Time) error {
	if notification.Type != "Notification" {
		return fmt.Errorf("unsupported SNS message type")
	}
	if notification.Message == "" || notification.MessageID == "" || notification.TopicArn == "" ||
		notification.Timestamp == "" || notification.Signature == "" || notification.SigningCertURL == "" ||
		(notification.SignatureVersion != "1" && notification.SignatureVersion != "2") {
		return fmt.Errorf("incomplete SNS notification")
	}
	timestamp, err := time.Parse(time.RFC3339, notification.Timestamp)
	if err != nil || timestamp.Before(now.Add(-5*time.Minute)) || timestamp.After(now.Add(5*time.Minute)) {
		return fmt.Errorf("stale SNS notification")
	}
	if !validCertificateURL(notification.SigningCertURL, region) {
		return fmt.Errorf("invalid SNS signing certificate URL")
	}
	if notification.SignatureVersion != "1" && notification.SignatureVersion != "2" {
		return fmt.Errorf("unsupported SNS signature version")
	}

	certificate, err := getCertificate(ctx, notification.SigningCertURL)
	if err != nil {
		return fmt.Errorf("load SNS signing certificate: %w", err)
	}
	publicKey, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("SNS signing certificate is not RSA")
	}
	signature, err := base64.StdEncoding.DecodeString(notification.Signature)
	if err != nil {
		return fmt.Errorf("decode SNS signature: %w", err)
	}
	message := []byte(StringToSign(notification))
	switch notification.SignatureVersion {
	case "1":
		digest := sha1.Sum(message) // #nosec G401 -- required by SNS SignatureVersion 1.
		return rsa.VerifyPKCS1v15(publicKey, crypto.SHA1, digest[:], signature)
	case "2":
		digest := sha256.Sum256(message)
		return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature)
	default:
		return fmt.Errorf("unsupported SNS signature version")
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
	if notification.Subject != "" {
		write("Subject", notification.Subject)
	}
	write("Timestamp", notification.Timestamp)
	write("TopicArn", notification.TopicArn)
	write("Type", notification.Type)
	return builder.String()
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
	defer response.Body.Close()
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
