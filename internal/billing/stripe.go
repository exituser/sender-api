package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

const stripeAPIBaseURL = "https://api.stripe.com/v1"
const stripeSignatureTolerance = 5 * time.Minute

type StripeClient struct {
	secretKey     string
	webhookSecret string
	prices        map[domain.Plan]string
	successURL    string
	cancelURL     string
	returnURL     string
	httpClient    *http.Client
	apiBaseURL    string
}

type CheckoutSession struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type PortalSession struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type Event struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created int64  `json:"created"`
	Data    struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

func NewStripeClient(secretKey, webhookSecret, proPrice, scalePrice, successURL, cancelURL, returnURL string) *StripeClient {
	return &StripeClient{
		secretKey:     strings.TrimSpace(secretKey),
		webhookSecret: strings.TrimSpace(webhookSecret),
		prices: map[domain.Plan]string{
			domain.PlanPro:   strings.TrimSpace(proPrice),
			domain.PlanScale: strings.TrimSpace(scalePrice),
		},
		successURL: strings.TrimSpace(successURL),
		cancelURL:  strings.TrimSpace(cancelURL),
		returnURL:  strings.TrimSpace(returnURL),
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBaseURL: stripeAPIBaseURL,
	}
}

func (c *StripeClient) Configured() bool {
	return c != nil && c.secretKey != ""
}

func (c *StripeClient) WebhookConfigured() bool {
	return c != nil && c.webhookSecret != ""
}

func (c *StripeClient) PlanForPrice(priceID string) (domain.Plan, bool) {
	if c != nil {
		for plan, configuredPrice := range c.prices {
			if configuredPrice != "" && configuredPrice == strings.TrimSpace(priceID) {
				return plan, true
			}
		}
	}
	return domain.PlanFree, false
}

func (c *StripeClient) CreateCustomer(ctx context.Context, team *domain.Team) (string, error) {
	if !c.Configured() {
		return "", errors.New("billing is not configured")
	}
	if team == nil {
		return "", errors.New("team is required")
	}
	form := url.Values{}
	form.Set("name", team.Name)
	form.Set("metadata[team_id]", team.ID.String())
	var response struct {
		ID string `json:"id"`
	}
	if err := c.postForm(ctx, "/customers", form, &response, "sender-api/customer/"+team.ID.String()); err != nil {
		return "", err
	}
	if response.ID == "" {
		return "", errors.New("stripe returned an empty customer id")
	}
	return response.ID, nil
}

func (c *StripeClient) CreateCheckoutSession(ctx context.Context, teamID uuid.UUID, customerID string, plan domain.Plan) (*CheckoutSession, error) {
	if !c.Configured() {
		return nil, errors.New("billing is not configured")
	}
	priceID := c.prices[plan]
	if priceID == "" {
		return nil, fmt.Errorf("stripe price is not configured for plan %q", plan)
	}
	if c.successURL == "" || c.cancelURL == "" {
		return nil, errors.New("billing success and cancel URLs are not configured")
	}
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("customer", customerID)
	form.Set("line_items[0][price]", priceID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("success_url", c.successURL)
	form.Set("cancel_url", c.cancelURL)
	form.Set("client_reference_id", teamID.String())
	form.Set("metadata[team_id]", teamID.String())
	form.Set("metadata[plan]", string(plan))
	form.Set("subscription_data[metadata][team_id]", teamID.String())
	form.Set("subscription_data[metadata][plan]", string(plan))
	var response CheckoutSession
	if err := c.postForm(ctx, "/checkout/sessions", form, &response, "sender-api/checkout/"+teamID.String()+"/"+string(plan)); err != nil {
		return nil, err
	}
	if response.ID == "" || response.URL == "" {
		return nil, errors.New("stripe returned an incomplete checkout session")
	}
	return &response, nil
}

func (c *StripeClient) CreatePortalSession(ctx context.Context, customerID string) (*PortalSession, error) {
	if !c.Configured() {
		return nil, errors.New("billing is not configured")
	}
	if customerID == "" || c.returnURL == "" {
		return nil, errors.New("billing customer and return URL are required")
	}
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("return_url", c.returnURL)
	var response PortalSession
	if err := c.postForm(ctx, "/billing_portal/sessions", form, &response, ""); err != nil {
		return nil, err
	}
	if response.ID == "" || response.URL == "" {
		return nil, errors.New("stripe returned an incomplete portal session")
	}
	return &response, nil
}

func (c *StripeClient) VerifyWebhook(payload []byte, signature string, now time.Time) (*Event, error) {
	if !c.WebhookConfigured() {
		return nil, errors.New("billing webhook is not configured")
	}
	var timestamp int64
	var signatures [][]byte
	for _, part := range strings.Split(signature, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, errors.New("invalid Stripe webhook timestamp")
			}
			timestamp = parsed
		case "v1":
			decoded, err := hex.DecodeString(value)
			if err == nil {
				signatures = append(signatures, decoded)
			}
		}
	}
	if timestamp == 0 || len(signatures) == 0 {
		return nil, errors.New("missing Stripe webhook signature")
	}
	signed := []byte(strconv.FormatInt(timestamp, 10) + "." + string(payload))
	hash := hmac.New(sha256.New, []byte(c.webhookSecret))
	_, _ = hash.Write(signed)
	expected := hash.Sum(nil)
	valid := false
	for _, candidate := range signatures {
		if len(candidate) == len(expected) && subtle.ConstantTimeCompare(candidate, expected) == 1 {
			valid = true
			break
		}
	}
	if !valid {
		return nil, errors.New("invalid Stripe webhook signature")
	}
	when := time.Unix(timestamp, 0)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if now.Sub(when) > stripeSignatureTolerance || when.Sub(now) > stripeSignatureTolerance {
		return nil, errors.New("stale Stripe webhook signature")
	}
	var event Event
	if err := json.Unmarshal(payload, &event); err != nil || event.ID == "" || event.Type == "" {
		return nil, errors.New("invalid Stripe webhook payload")
	}
	return &event, nil
}

func (c *StripeClient) postForm(ctx context.Context, path string, form url.Values, output any, idempotencyKey string) error {
	body := []byte(form.Encode())
	baseURL := c.apiBaseURL
	if baseURL == "" {
		baseURL = stripeAPIBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("stripe API request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read Stripe API response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("stripe API returned %s: %s", response.Status, strings.TrimSpace(string(responseBody)))
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("decode Stripe API response: %w", err)
	}
	return nil
}
