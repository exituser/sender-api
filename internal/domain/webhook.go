package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Webhook struct {
	ID        uuid.UUID `json:"id" db:"id"`
	TeamID    uuid.UUID `json:"team_id" db:"team_id"`
	URL       string    `json:"url" db:"url"`
	Events    []string  `json:"events" db:"events"`
	Secret    string    `json:"-" db:"secret"`
	Active    bool      `json:"active" db:"active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type WebhookEvent struct {
	WebhookID uuid.UUID       `json:"webhook_id"`
	Event     string          `json:"event"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

type CreateWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

type UpdateWebhookRequest struct {
	URL    *string   `json:"url,omitempty"`
	Events *[]string `json:"events,omitempty"`
	Active *bool     `json:"active,omitempty"`
}

type WebhookListResponse struct {
	Data []Webhook `json:"data"`
}

type CreateWebhookResponse struct {
	ID        uuid.UUID `json:"id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	Secret    string    `json:"secret"`
	CreatedAt time.Time `json:"created_at"`
}
