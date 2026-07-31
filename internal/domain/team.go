package domain

import (
	"time"

	"github.com/google/uuid"
)

type Plan string

const (
	PlanFree  Plan = "free"
	PlanPro   Plan = "pro"
	PlanScale Plan = "scale"
)

type Team struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	Name                 string     `json:"name" db:"name"`
	Slug                 string     `json:"slug" db:"slug"`
	Plan                 Plan       `json:"plan" db:"plan"`
	StripeCustomerID     *string    `json:"stripe_customer_id,omitempty" db:"stripe_customer_id"`
	StripeSubscriptionID *string    `json:"stripe_subscription_id,omitempty" db:"stripe_subscription_id"`
	BillingStatus        string     `json:"billing_status" db:"billing_status"`
	CurrentPeriodEnd     *time.Time `json:"current_period_end,omitempty" db:"current_period_end"`
	CancelAtPeriodEnd    bool       `json:"cancel_at_period_end" db:"cancel_at_period_end"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}

type TeamMemberRole string

const (
	TeamMemberRoleOwner  TeamMemberRole = "owner"
	TeamMemberRoleAdmin  TeamMemberRole = "admin"
	TeamMemberRoleMember TeamMemberRole = "member"
)

type TeamMember struct {
	ID        uuid.UUID      `json:"id" db:"id"`
	TeamID    uuid.UUID      `json:"team_id" db:"team_id"`
	UserID    uuid.UUID      `json:"user_id" db:"user_id"`
	Role      TeamMemberRole `json:"role" db:"role"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
}

type CreateTeamRequest struct {
	Name string `json:"name"`
}

type UpdateTeamRequest struct {
	Name *string `json:"name,omitempty"`
}

type InviteMemberRequest struct {
	Email string         `json:"email"`
	Role  TeamMemberRole `json:"role"`
}

type TeamInvitationStatus string

const (
	TeamInvitationStatusPending  TeamInvitationStatus = "pending"
	TeamInvitationStatusAccepted TeamInvitationStatus = "accepted"
	TeamInvitationStatusRevoked  TeamInvitationStatus = "revoked"
	TeamInvitationStatusExpired  TeamInvitationStatus = "expired"
)

type TeamInvitation struct {
	ID               uuid.UUID            `json:"id" db:"id"`
	TeamID           uuid.UUID            `json:"team_id" db:"team_id"`
	Email            string               `json:"email" db:"email"`
	Role             TeamMemberRole       `json:"role" db:"role"`
	TokenHash        string               `json:"-" db:"token_hash"`
	Status           TeamInvitationStatus `json:"status" db:"status"`
	ExpiresAt        time.Time            `json:"expires_at" db:"expires_at"`
	AcceptedAt       *time.Time           `json:"accepted_at,omitempty" db:"accepted_at"`
	AcceptedByUserID *uuid.UUID           `json:"accepted_by_user_id,omitempty" db:"accepted_by_user_id"`
	RevokedAt        *time.Time           `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedAt        time.Time            `json:"created_at" db:"created_at"`
}

type CreateInvitationResponse struct {
	Invitation TeamInvitation `json:"invitation"`
	Token      string         `json:"token"`
}

type AcceptInvitationRequest struct {
	Token string `json:"token"`
}

type TeamMemberResponse struct {
	ID        uuid.UUID      `json:"id"`
	UserID    uuid.UUID      `json:"user_id"`
	Email     string         `json:"email"`
	Role      TeamMemberRole `json:"role"`
	CreatedAt time.Time      `json:"created_at"`
}
