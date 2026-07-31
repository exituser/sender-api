package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type InboundEmail struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	TeamID      uuid.UUID       `json:"team_id" db:"team_id"`
	MessageID   *string         `json:"message_id,omitempty" db:"message_id"`
	From        string          `json:"from" db:"from_addr"`
	To          []string        `json:"to" db:"to_addr"`
	Subject     *string         `json:"subject,omitempty" db:"subject"`
	Text        *string         `json:"text,omitempty" db:"text"`
	HTML        *string         `json:"html,omitempty" db:"html"`
	Attachments json.RawMessage `json:"attachments,omitempty" db:"attachments"`
	RawS3Key    *string         `json:"raw_s3_key,omitempty" db:"raw_s3_key"`
	Headers     json.RawMessage `json:"headers,omitempty" db:"headers"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
}

type InboundAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id,omitempty"`
	Content     []byte `json:"content"`
}

type InboundEmailListResponse struct {
	Data   []InboundEmail `json:"data"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}
