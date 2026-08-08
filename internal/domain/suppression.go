package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type SuppressionReason string

const (
	SuppressionReasonBounce      SuppressionReason = "bounce"
	SuppressionReasonComplaint   SuppressionReason = "complaint"
	SuppressionReasonUnsubscribe SuppressionReason = "unsubscribe"
)

type Suppression struct {
	ID        uuid.UUID         `json:"id" db:"id"`
	TeamID    uuid.UUID         `json:"team_id" db:"team_id"`
	Email     string            `json:"email" db:"email"`
	Reason    SuppressionReason `json:"reason" db:"reason"`
	CreatedAt time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt time.Time         `json:"updated_at" db:"updated_at"`
}

// NormalizeEmail produces the canonical value used by suppression lookups.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
