package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

type TeamRepo struct {
	db *pgxpool.Pool
}

func NewTeamRepo(db *pgxpool.Pool) *TeamRepo {
	return &TeamRepo{db: db}
}

func (r *TeamRepo) Create(ctx context.Context, team *domain.Team) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO teams (id, name, slug, plan)
		VALUES ($1, $2, $3, $4)
	`, team.ID, team.Name, team.Slug, team.Plan)
	return err
}

func (r *TeamRepo) CreateWithOwner(ctx context.Context, team *domain.Team, member *domain.TeamMember) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO teams (id, name, slug, plan)
		VALUES ($1, $2, $3, $4)
	`, team.ID, team.Name, team.Slug, team.Plan); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO team_members (id, team_id, user_id, role)
		VALUES ($1, $2, $3, $4)
	`, member.ID, member.TeamID, member.UserID, member.Role); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *TeamRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
	var team domain.Team
	err := r.db.QueryRow(ctx, `
		SELECT id, name, slug, plan, stripe_customer_id, stripe_subscription_id, created_at, updated_at
		FROM teams WHERE id = $1
	`, id).Scan(
		&team.ID, &team.Name, &team.Slug, &team.Plan,
		&team.StripeCustomerID, &team.StripeSubscriptionID,
		&team.CreatedAt, &team.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (r *TeamRepo) GetBySlug(ctx context.Context, slug string) (*domain.Team, error) {
	var team domain.Team
	err := r.db.QueryRow(ctx, `
		SELECT id, name, slug, plan, created_at, updated_at
		FROM teams WHERE slug = $1
	`, slug).Scan(&team.ID, &team.Name, &team.Slug, &team.Plan, &team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (r *TeamRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Team, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.name, t.slug, t.plan, t.created_at, t.updated_at
		FROM teams t
		JOIN team_members tm ON t.id = tm.team_id
		WHERE tm.user_id = $1
		ORDER BY t.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query teams: %w", err)
	}
	defer rows.Close()

	var teams []domain.Team
	for rows.Next() {
		var team domain.Team
		err := rows.Scan(&team.ID, &team.Name, &team.Slug, &team.Plan, &team.CreatedAt, &team.UpdatedAt)
		if err != nil {
			return nil, err
		}
		teams = append(teams, team)
	}
	return teams, nil
}

func (r *TeamRepo) Update(ctx context.Context, team *domain.Team) error {
	_, err := r.db.Exec(ctx, `
		UPDATE teams SET name = $1, updated_at = NOW() WHERE id = $2
	`, team.Name, team.ID)
	return err
}

func (r *TeamRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM teams WHERE id = $1`, id)
	return err
}

func (r *TeamRepo) AddMember(ctx context.Context, member *domain.TeamMember) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO team_members (id, team_id, user_id, role)
		VALUES ($1, $2, $3, $4)
	`, member.ID, member.TeamID, member.UserID, member.Role)
	return err
}

func (r *TeamRepo) GetUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.db.QueryRow(ctx, `SELECT id FROM auth.users WHERE lower(email) = lower($1)`, email).Scan(&userID)
	return userID, err
}

func (r *TeamRepo) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`, teamID, userID)
	return err
}

func (r *TeamRepo) UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role domain.TeamMemberRole) error {
	_, err := r.db.Exec(ctx, `UPDATE team_members SET role = $1 WHERE team_id = $2 AND user_id = $3`, role, teamID, userID)
	return err
}

func (r *TeamRepo) GetMembers(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMemberResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT tm.id, tm.user_id, u.email, tm.role, tm.created_at
		FROM team_members tm
		JOIN auth.users u ON tm.user_id = u.id
		WHERE tm.team_id = $1
		ORDER BY tm.created_at
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()

	var members []domain.TeamMemberResponse
	for rows.Next() {
		var m domain.TeamMemberResponse
		err := rows.Scan(&m.ID, &m.UserID, &m.Email, &m.Role, &m.CreatedAt)
		if err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *TeamRepo) GetMember(ctx context.Context, teamID, userID uuid.UUID) (*domain.TeamMember, error) {
	var member domain.TeamMember
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, user_id, role, created_at
		FROM team_members WHERE team_id = $1 AND user_id = $2
	`, teamID, userID).Scan(&member.ID, &member.TeamID, &member.UserID, &member.Role, &member.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &member, nil
}
