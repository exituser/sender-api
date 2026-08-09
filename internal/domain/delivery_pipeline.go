package domain

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrDeliveryConfirmationUnavailable = errors.New("verified delivery confirmation is unavailable")
var ErrProviderCorrelationMismatch = errors.New("provider event does not match the correlated email")

type RetentionClass string

const (
	RetentionOutbound RetentionClass = "outbound"
	RetentionInbound  RetentionClass = "inbound"
)

// WebhookOutboxEvent is the durable, endpoint-independent webhook work item
// committed in the same PostgreSQL transaction as its domain event.
type WebhookOutboxEvent struct {
	ID             uuid.UUID
	TeamID         uuid.UUID
	EventID        uuid.UUID
	Event          string
	Payload        json.RawMessage
	RetentionClass RetentionClass
	CreatedAt      time.Time
}

type ProviderEventInbox struct {
	EventID           uuid.UUID
	ProviderMessageID string
	EventType         string
	Payload           json.RawMessage
	EmailID           *uuid.UUID
	SendAttemptID     *uuid.UUID
	Attempts          int
	CreatedAt         time.Time
}

type SendAttemptClaim struct {
	EmailID    uuid.UUID
	AttemptID  uuid.UUID
	FenceToken uuid.UUID
	LeaseUntil time.Time
}
