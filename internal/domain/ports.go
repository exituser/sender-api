package domain

import (
	"context"

	"github.com/google/uuid"
)

type EmailRepository interface {
	Create(ctx context.Context, email *Email) error
	GetByID(ctx context.Context, id uuid.UUID) (*Email, error)
	GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*Email, error)
	List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*EmailListResponse, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status EmailStatus) error
	UpdateStatusForTeam(ctx context.Context, teamID, id uuid.UUID, status EmailStatus) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
	AddEvent(ctx context.Context, event *EmailEvent) error
	GetEvents(ctx context.Context, emailID uuid.UUID) ([]EmailEvent, error)
	GetEventsForTeam(ctx context.Context, teamID, emailID uuid.UUID) ([]EmailEvent, error)
}

type TeamRepository interface {
	Create(ctx context.Context, team *Team) error
	GetByID(ctx context.Context, id uuid.UUID) (*Team, error)
	GetBySlug(ctx context.Context, slug string) (*Team, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Team, error)
	Update(ctx context.Context, team *Team) error
	Delete(ctx context.Context, id uuid.UUID) error
	AddMember(ctx context.Context, member *TeamMember) error
	GetUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error)
	RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error
	UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role TeamMemberRole) error
	GetMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMemberResponse, error)
	GetMember(ctx context.Context, teamID, userID uuid.UUID) (*TeamMember, error)
}

type ContactRepository interface {
	Create(ctx context.Context, contact *Contact) error
	GetByID(ctx context.Context, id uuid.UUID) (*Contact, error)
	GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*Contact, error)
	GetByEmail(ctx context.Context, teamID uuid.UUID, email string) (*Contact, error)
	List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*ContactListResponse, error)
	Update(ctx context.Context, contact *Contact) error
	UpdateForTeam(ctx context.Context, contact *Contact) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
	BulkCreate(ctx context.Context, contacts []*Contact) error
}

type DomainRepository interface {
	Create(ctx context.Context, domain *Domain) error
	GetByID(ctx context.Context, id uuid.UUID) (*Domain, error)
	GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*Domain, error)
	GetByName(ctx context.Context, teamID uuid.UUID, name string) (*Domain, error)
	List(ctx context.Context, teamID uuid.UUID) (*DomainListResponse, error)
	Update(ctx context.Context, domain *Domain) error
	UpdateForTeam(ctx context.Context, domain *Domain) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
	GetTeamByDomain(ctx context.Context, domainName string) (uuid.UUID, error)
}

type APIKeyRepository interface {
	Create(ctx context.Context, key *APIKey) error
	GetByID(ctx context.Context, id uuid.UUID) (*APIKey, error)
	GetByHash(ctx context.Context, hash string) (*APIKey, error)
	List(ctx context.Context, teamID uuid.UUID) ([]APIKey, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
}

type InboundEmailRepository interface {
	Create(ctx context.Context, email *InboundEmail) error
	GetByID(ctx context.Context, id uuid.UUID) (*InboundEmail, error)
	List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*InboundEmailListResponse, error)
}

type WebhookRepository interface {
	Create(ctx context.Context, webhook *Webhook) error
	GetByID(ctx context.Context, id uuid.UUID) (*Webhook, error)
	GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*Webhook, error)
	List(ctx context.Context, teamID uuid.UUID) (*WebhookListResponse, error)
	Update(ctx context.Context, webhook *Webhook) error
	UpdateForTeam(ctx context.Context, webhook *Webhook) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
	GetByEvent(ctx context.Context, teamID uuid.UUID, event string) ([]Webhook, error)
}

type EmailSender interface {
	Send(ctx context.Context, email *Email) error
}

type EmailQueue interface {
	Enqueue(ctx context.Context, emailID string) error
	Dequeue(ctx context.Context) (string, error)
	Ack(ctx context.Context, emailID string) error
	Requeue(ctx context.Context, emailID string, countAttempt bool) error
	Recover(ctx context.Context) error
}
