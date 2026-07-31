package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
)

const teamInvitationTTL = 7 * 24 * time.Hour

type TeamService struct {
	teamRepo domain.TeamRepository
	logger   *slog.Logger
}

func NewTeamService(teamRepo domain.TeamRepository, logger *slog.Logger) *TeamService {
	if logger == nil {
		logger = slog.Default()
	}
	return &TeamService{
		teamRepo: teamRepo,
		logger:   logger,
	}
}

func (s *TeamService) Create(ctx context.Context, userID uuid.UUID, req *domain.CreateTeamRequest) (*domain.Team, error) {
	if req == nil {
		return nil, fmt.Errorf("team request is required")
	}
	if userID == uuid.Nil {
		return nil, fmt.Errorf("user id is required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 255 {
		return nil, fmt.Errorf("team name must be between 1 and 255 characters")
	}
	slug := s.generateSlug(name)
	if !validator.IsValidSlug(slug) {
		return nil, fmt.Errorf("team name must produce a valid slug")
	}

	team := &domain.Team{
		ID:   uuid.New(),
		Name: name,
		Slug: slug,
		Plan: domain.PlanFree,
	}

	member := &domain.TeamMember{
		ID:     uuid.New(),
		TeamID: team.ID,
		UserID: userID,
		Role:   domain.TeamMemberRoleOwner,
	}

	if err := s.teamRepo.CreateWithOwner(ctx, team, member); err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	s.logger.Info("team created", "team_id", team.ID, "name", team.Name)
	return team, nil
}

func (s *TeamService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
	return s.teamRepo.GetByID(ctx, id)
}

func (s *TeamService) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Team, error) {
	return s.teamRepo.ListByUser(ctx, userID)
}

func (s *TeamService) Update(ctx context.Context, id uuid.UUID, req *domain.UpdateTeamRequest) (*domain.Team, error) {
	if req == nil {
		return nil, fmt.Errorf("team request is required")
	}
	team, err := s.teamRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 255 {
			return nil, fmt.Errorf("team name must be between 1 and 255 characters")
		}
		team.Name = name
	}

	if err := s.teamRepo.Update(ctx, team); err != nil {
		return nil, fmt.Errorf("failed to update team: %w", err)
	}

	return team, nil
}

func (s *TeamService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.teamRepo.Delete(ctx, id)
}

func (s *TeamService) CreateInvitation(ctx context.Context, teamID uuid.UUID, req *domain.InviteMemberRequest) (*domain.CreateInvitationResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("invitation request is required")
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !validator.IsValidEmail(email) {
		return nil, fmt.Errorf("invalid invitation email")
	}
	if req.Role != domain.TeamMemberRoleAdmin && req.Role != domain.TeamMemberRoleMember {
		return nil, fmt.Errorf("members can only be invited as admin or member")
	}
	token, err := newInvitationToken()
	if err != nil {
		return nil, fmt.Errorf("generate invitation token: %w", err)
	}
	tokenHash := sha256.Sum256([]byte(token))
	invitation := &domain.TeamInvitation{
		ID:        uuid.New(),
		TeamID:    teamID,
		Email:     email,
		Role:      req.Role,
		TokenHash: fmt.Sprintf("%x", tokenHash),
		Status:    domain.TeamInvitationStatusPending,
		ExpiresAt: time.Now().UTC().Add(teamInvitationTTL),
	}
	if err := s.teamRepo.CreateInvitation(ctx, invitation); err != nil {
		return nil, fmt.Errorf("failed to create invitation: %w", err)
	}
	s.logger.Info("team invitation created", "team_id", teamID, "role", req.Role)
	return &domain.CreateInvitationResponse{Invitation: *invitation, Token: token}, nil
}

func (s *TeamService) ListInvitations(ctx context.Context, teamID uuid.UUID) ([]domain.TeamInvitation, error) {
	return s.teamRepo.ListInvitations(ctx, teamID)
}

func (s *TeamService) RevokeInvitation(ctx context.Context, teamID, invitationID uuid.UUID) error {
	return s.teamRepo.RevokeInvitation(ctx, teamID, invitationID)
}

func (s *TeamService) AcceptInvitation(ctx context.Context, token string, userID uuid.UUID) (*domain.TeamInvitation, error) {
	if userID == uuid.Nil || strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("invitation is invalid or expired")
	}
	tokenHash := sha256.Sum256([]byte(token))
	invitation, err := s.teamRepo.AcceptInvitation(ctx, fmt.Sprintf("%x", tokenHash), userID)
	if err != nil {
		return nil, fmt.Errorf("invitation is invalid or expired")
	}
	return invitation, nil
}

func newInvitationToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (s *TeamService) GetMember(ctx context.Context, teamID, userID uuid.UUID) (*domain.TeamMember, error) {
	return s.teamRepo.GetMember(ctx, teamID, userID)
}

func (s *TeamService) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	member, err := s.teamRepo.GetMember(ctx, teamID, userID)
	if err != nil {
		return err
	}
	if member.Role == domain.TeamMemberRoleOwner {
		return fmt.Errorf("the team owner cannot be removed")
	}
	return s.teamRepo.RemoveMember(ctx, teamID, userID)
}

func (s *TeamService) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role domain.TeamMemberRole) error {
	if role != domain.TeamMemberRoleAdmin && role != domain.TeamMemberRoleMember {
		return fmt.Errorf("member role must be admin or member")
	}
	member, err := s.teamRepo.GetMember(ctx, teamID, userID)
	if err != nil {
		return err
	}
	if member.Role == domain.TeamMemberRoleOwner {
		return fmt.Errorf("the team owner role cannot be changed")
	}
	return s.teamRepo.UpdateMemberRole(ctx, teamID, userID, role)
}

func (s *TeamService) GetMembers(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMemberResponse, error) {
	return s.teamRepo.GetMembers(ctx, teamID)
}

func (s *TeamService) generateSlug(name string) string {
	var builder strings.Builder
	separatorPending := false
	for _, char := range strings.ToLower(strings.TrimSpace(name)) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			if separatorPending && builder.Len() > 0 {
				builder.WriteByte('-')
			}
			builder.WriteRune(char)
			separatorPending = false
			continue
		}
		if builder.Len() > 0 {
			separatorPending = true
		}
	}
	return builder.String()
}
