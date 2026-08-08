package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type EmailStatus string

const (
	EmailStatusQueued     EmailStatus = "queued"
	EmailStatusSending    EmailStatus = "sending"
	EmailStatusSent       EmailStatus = "sent"
	EmailStatusDelivered  EmailStatus = "delivered"
	EmailStatusOpened     EmailStatus = "opened"
	EmailStatusClicked    EmailStatus = "clicked"
	EmailStatusBounced    EmailStatus = "bounced"
	EmailStatusComplained EmailStatus = "complained"
	EmailStatusFailed     EmailStatus = "failed"
	EmailStatusCancelled  EmailStatus = "cancelled"
)

type EmailCategory string

const (
	EmailCategoryTransactional EmailCategory = "transactional"
	EmailCategoryMarketing     EmailCategory = "marketing"
)

type Email struct {
	ID                uuid.UUID         `json:"id" db:"id"`
	TeamID            uuid.UUID         `json:"team_id" db:"team_id"`
	APIKeyID          *uuid.UUID        `json:"api_key_id,omitempty" db:"api_key_id"`
	From              string            `json:"from" db:"from_addr"`
	To                []string          `json:"to" db:"to_addr"`
	CC                []string          `json:"cc,omitempty" db:"cc"`
	BCC               []string          `json:"bcc,omitempty" db:"bcc"`
	Subject           string            `json:"subject" db:"subject"`
	Category          EmailCategory     `json:"category" db:"category"`
	HTML              string            `json:"html,omitempty" db:"html"`
	Text              string            `json:"text,omitempty" db:"text"`
	ReplyTo           []string          `json:"reply_to,omitempty" db:"reply_to"`
	Attachments       []Attachment      `json:"attachments,omitempty" db:"attachments"`
	Status            EmailStatus       `json:"status" db:"status"`
	Tags              []Tag             `json:"tags,omitempty" db:"tags"`
	Metadata          map[string]string `json:"metadata,omitempty" db:"metadata"`
	Headers           map[string]string `json:"headers,omitempty" db:"headers"`
	IdempotencyKey    *string           `json:"-" db:"idempotency_key"`
	IdempotencyHash   *string           `json:"-" db:"idempotency_hash"`
	ProviderMessageID *string           `json:"-" db:"provider_message_id"`
	SendingAt         *time.Time        `json:"-" db:"sending_at"`
	ScheduledAt       *time.Time        `json:"scheduled_at,omitempty" db:"scheduled_at"`
	SentAt            *time.Time        `json:"sent_at,omitempty" db:"sent_at"`
	CreatedAt         time.Time         `json:"created_at" db:"created_at"`
}

type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SendEmailRequest struct {
	From        string            `json:"from"`
	To          []string          `json:"to"`
	CC          []string          `json:"cc,omitempty"`
	BCC         []string          `json:"bcc,omitempty"`
	Subject     string            `json:"subject"`
	Category    EmailCategory     `json:"category,omitempty"`
	HTML        string            `json:"html,omitempty"`
	Text        string            `json:"text,omitempty"`
	ReplyTo     []string          `json:"reply_to,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Tags        []Tag             `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
}

type Attachment struct {
	Filename string `json:"filename"`
	Content  []byte `json:"content,omitempty"`
}

type EmailResponse struct {
	ID         string `json:"id"`
	Idempotent bool   `json:"idempotent,omitempty"`
}

type DeadLetter struct {
	ID     string      `json:"id"`
	Status EmailStatus `json:"status"`
}

type EmailListResponse struct {
	Data   []Email `json:"data"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type EmailEvent struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	EmailID   uuid.UUID       `json:"email_id" db:"email_id"`
	Event     string          `json:"event" db:"event"`
	Timestamp time.Time       `json:"timestamp" db:"timestamp"`
	Data      json.RawMessage `json:"data,omitempty" db:"data"`
	IPAddress string          `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent string          `json:"user_agent,omitempty" db:"user_agent"`
}
