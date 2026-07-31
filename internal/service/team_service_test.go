package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

func TestGenerateSlugNormalizesSeparators(t *testing.T) {
	service := &TeamService{}
	if got := service.generateSlug("  Acme__Mail  "); got != "acme-mail" {
		t.Fatalf("expected normalized slug, got %q", got)
	}
}

func TestCreateInvitationStoresOnlyTokenHashAndNormalizesEmail(t *testing.T) {
	repo := &teamInvitationTestRepo{}
	service := NewTeamService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

	response, err := service.CreateInvitation(context.Background(), uuid.New(), &domain.InviteMemberRequest{
		Email: "  TeSt@Example.COM ",
		Role:  domain.TeamMemberRoleMember,
	})
	if err != nil {
		t.Fatalf("CreateInvitation() error = %v", err)
	}
	if response.Token == "" || repo.created.TokenHash == "" {
		t.Fatal("expected an invitation token and stored token hash")
	}
	if response.Token == repo.created.TokenHash {
		t.Fatal("raw invitation token must not be stored")
	}
	if response.Invitation.Email != "test@example.com" {
		t.Fatalf("expected normalized email, got %q", response.Invitation.Email)
	}
	if response.Invitation.Status != domain.TeamInvitationStatusPending {
		t.Fatalf("expected pending invitation, got %q", response.Invitation.Status)
	}
	if remaining := time.Until(response.Invitation.ExpiresAt); remaining < teamInvitationTTL-time.Minute || remaining > teamInvitationTTL {
		t.Fatalf("unexpected invitation expiry: %s", remaining)
	}
}

func TestAcceptInvitationHashesTokenAndHidesRepositoryDetails(t *testing.T) {
	repo := &teamInvitationTestRepo{acceptErr: errors.New("email does not match")}
	service := NewTeamService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

	_, err := service.AcceptInvitation(context.Background(), "raw-token", uuid.New())
	if err == nil || err.Error() != "invitation is invalid or expired" {
		t.Fatalf("expected generic invalid invitation error, got %v", err)
	}
	if repo.acceptedTokenHash == "" || repo.acceptedTokenHash == "raw-token" {
		t.Fatal("expected accept to pass only a token hash to the repository")
	}
}

type teamInvitationTestRepo struct {
	created           domain.TeamInvitation
	acceptedTokenHash string
	acceptErr         error
}

func (r *teamInvitationTestRepo) CreateInvitation(_ context.Context, invitation *domain.TeamInvitation) error {
	r.created = *invitation
	return nil
}

func (r *teamInvitationTestRepo) AcceptInvitation(_ context.Context, tokenHash string, _ uuid.UUID) (*domain.TeamInvitation, error) {
	r.acceptedTokenHash = tokenHash
	return nil, r.acceptErr
}

func (r *teamInvitationTestRepo) Create(context.Context, *domain.Team) error { return nil }
func (r *teamInvitationTestRepo) CreateWithOwner(context.Context, *domain.Team, *domain.TeamMember) error {
	return nil
}
func (r *teamInvitationTestRepo) GetByID(context.Context, uuid.UUID) (*domain.Team, error) {
	return nil, nil
}
func (r *teamInvitationTestRepo) GetBySlug(context.Context, string) (*domain.Team, error) {
	return nil, nil
}
func (r *teamInvitationTestRepo) ListByUser(context.Context, uuid.UUID) ([]domain.Team, error) {
	return nil, nil
}
func (r *teamInvitationTestRepo) Update(context.Context, *domain.Team) error { return nil }
func (r *teamInvitationTestRepo) Delete(context.Context, uuid.UUID) error    { return nil }
func (r *teamInvitationTestRepo) AddMember(context.Context, *domain.TeamMember) error {
	return nil
}
func (r *teamInvitationTestRepo) GetUserIDByEmail(context.Context, string) (uuid.UUID, error) {
	return uuid.Nil, nil
}
func (r *teamInvitationTestRepo) RemoveMember(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (r *teamInvitationTestRepo) UpdateMemberRole(context.Context, uuid.UUID, uuid.UUID, domain.TeamMemberRole) error {
	return nil
}
func (r *teamInvitationTestRepo) GetMembers(context.Context, uuid.UUID) ([]domain.TeamMemberResponse, error) {
	return nil, nil
}
func (r *teamInvitationTestRepo) GetMember(context.Context, uuid.UUID, uuid.UUID) (*domain.TeamMember, error) {
	return nil, nil
}
func (r *teamInvitationTestRepo) ListInvitations(context.Context, uuid.UUID) ([]domain.TeamInvitation, error) {
	return nil, nil
}
func (r *teamInvitationTestRepo) RevokeInvitation(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
