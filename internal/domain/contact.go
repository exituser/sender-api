package domain

import (
	"time"

	"github.com/google/uuid"
)

type Contact struct {
	ID         uuid.UUID         `json:"id" db:"id"`
	TeamID     uuid.UUID         `json:"team_id" db:"team_id"`
	Email      string            `json:"email" db:"email"`
	FirstName  *string           `json:"first_name,omitempty" db:"first_name"`
	LastName   *string           `json:"last_name,omitempty" db:"last_name"`
	Subscribed bool              `json:"subscribed" db:"subscribed"`
	Properties map[string]string `json:"properties,omitempty" db:"properties"`
	CreatedAt  time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at" db:"updated_at"`
}

type CreateContactRequest struct {
	Email      string            `json:"email"`
	FirstName  *string           `json:"first_name,omitempty"`
	LastName   *string           `json:"last_name,omitempty"`
	Subscribed *bool             `json:"subscribed,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type UpdateContactRequest struct {
	Email      *string           `json:"email,omitempty"`
	FirstName  *string           `json:"first_name,omitempty"`
	LastName   *string           `json:"last_name,omitempty"`
	Subscribed *bool             `json:"subscribed,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type ContactListResponse struct {
	Data   []Contact `json:"data"`
	Total  int       `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}
