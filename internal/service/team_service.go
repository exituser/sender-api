package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
)

type TeamService struct {
	teamRepo domain.TeamRepository
	logger   *slog.Logger
}

func NewTeamService(teamRepo domain.TeamRepository, logger *slog.Logger) *TeamService {
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

	if err := s.teamRepo.Create(ctx, team); err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	member := &domain.TeamMember{
		ID:     uuid.New(),
		TeamID: team.ID,
		UserID: userID,
		Role:   domain.TeamMemberRoleOwner,
	}

	if err := s.teamRepo.AddMember(ctx, member); err != nil {
		s.teamRepo.Delete(ctx, team.ID)
		return nil, fmt.Errorf("failed to add owner: %w", err)
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

func (s *TeamService) AddMember(ctx context.Context, teamID uuid.UUID, req *domain.InviteMemberRequest) (*domain.TeamMember, error) {
	if req == nil {
		return nil, fmt.Errorf("member request is required")
	}
	if !validator.IsValidEmail(req.Email) {
		return nil, fmt.Errorf("invalid member email")
	}
	if req.Role != domain.TeamMemberRoleAdmin && req.Role != domain.TeamMemberRoleMember {
		return nil, fmt.Errorf("members can only be invited as admin or member")
	}
	userID, err := s.teamRepo.GetUserIDByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("user must sign up before being invited")
	}
	member := &domain.TeamMember{
		ID:     uuid.New(),
		TeamID: teamID,
		UserID: userID,
		Role:   req.Role,
	}

	if err := s.teamRepo.AddMember(ctx, member); err != nil {
		return nil, fmt.Errorf("failed to add member: %w", err)
	}

	s.logger.Info("member added", "team_id", teamID, "role", req.Role)
	return member, nil
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
