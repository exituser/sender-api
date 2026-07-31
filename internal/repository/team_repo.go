package repository

import (
	"context"
	"fmt"
	"time"

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
		SELECT id, name, slug, plan, stripe_customer_id, stripe_subscription_id, billing_status, current_period_end, cancel_at_period_end, created_at, updated_at
		FROM teams WHERE id = $1
	`, id).Scan(
		&team.ID, &team.Name, &team.Slug, &team.Plan,
		&team.StripeCustomerID, &team.StripeSubscriptionID, &team.BillingStatus, &team.CurrentPeriodEnd, &team.CancelAtPeriodEnd,
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
		SELECT id, name, slug, plan, billing_status, current_period_end, cancel_at_period_end, created_at, updated_at
		FROM teams WHERE slug = $1
	`, slug).Scan(&team.ID, &team.Name, &team.Slug, &team.Plan, &team.BillingStatus, &team.CurrentPeriodEnd, &team.CancelAtPeriodEnd, &team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (r *TeamRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Team, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.name, t.slug, t.plan, t.billing_status, t.current_period_end, t.cancel_at_period_end, t.created_at, t.updated_at
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
		err := rows.Scan(&team.ID, &team.Name, &team.Slug, &team.Plan, &team.BillingStatus, &team.CurrentPeriodEnd, &team.CancelAtPeriodEnd, &team.CreatedAt, &team.UpdatedAt)
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

func (r *TeamRepo) SetStripeCustomerID(ctx context.Context, teamID uuid.UUID, customerID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE teams SET stripe_customer_id = $1, updated_at = NOW() WHERE id = $2
	`, customerID, teamID)
	return err
}

func (r *TeamRepo) GetByStripeCustomerID(ctx context.Context, customerID string) (*domain.Team, error) {
	var team domain.Team
	err := r.db.QueryRow(ctx, `
		SELECT id, name, slug, plan, stripe_customer_id, stripe_subscription_id, billing_status, current_period_end, cancel_at_period_end, created_at, updated_at
		FROM teams WHERE stripe_customer_id = $1
	`, customerID).Scan(
		&team.ID, &team.Name, &team.Slug, &team.Plan,
		&team.StripeCustomerID, &team.StripeSubscriptionID, &team.BillingStatus, &team.CurrentPeriodEnd, &team.CancelAtPeriodEnd,
		&team.CreatedAt, &team.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (r *TeamRepo) GetByStripeSubscriptionID(ctx context.Context, subscriptionID string) (*domain.Team, error) {
	var team domain.Team
	err := r.db.QueryRow(ctx, `
		SELECT id, name, slug, plan, stripe_customer_id, stripe_subscription_id, billing_status, current_period_end, cancel_at_period_end, created_at, updated_at
		FROM teams WHERE stripe_subscription_id = $1
	`, subscriptionID).Scan(
		&team.ID, &team.Name, &team.Slug, &team.Plan,
		&team.StripeCustomerID, &team.StripeSubscriptionID, &team.BillingStatus, &team.CurrentPeriodEnd, &team.CancelAtPeriodEnd,
		&team.CreatedAt, &team.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &team, nil
}

func (r *TeamRepo) UpdateBilling(ctx context.Context, teamID uuid.UUID, customerID, subscriptionID *string, plan domain.Plan, status string, currentPeriodEnd *time.Time, cancelAtPeriodEnd bool) error {
	_, err := r.db.Exec(ctx, `
		UPDATE teams
		SET stripe_customer_id = $1,
			stripe_subscription_id = $2,
			plan = $3,
			billing_status = $4,
			current_period_end = $5,
			cancel_at_period_end = $6,
			updated_at = NOW()
		WHERE id = $7
	`, customerID, subscriptionID, plan, status, currentPeriodEnd, cancelAtPeriodEnd, teamID)
	return err
}

func (r *TeamRepo) CreateInvitation(ctx context.Context, invitation *domain.TeamInvitation) error {
	if _, err := r.db.Exec(ctx, `
		UPDATE team_invitations
		SET status = 'revoked', revoked_at = NOW()
		WHERE team_id = $1 AND lower(email) = lower($2)
			AND status = 'pending' AND expires_at <= NOW()
	`, invitation.TeamID, invitation.Email); err != nil {
		return err
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO team_invitations (id, team_id, email, role, token_hash, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, invitation.ID, invitation.TeamID, invitation.Email, invitation.Role, invitation.TokenHash, invitation.Status, invitation.ExpiresAt)
	return err
}

func (r *TeamRepo) ListInvitations(ctx context.Context, teamID uuid.UUID) ([]domain.TeamInvitation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, email, role,
			CASE WHEN status = 'pending' AND expires_at <= NOW() THEN 'expired' ELSE status END,
			expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at
		FROM team_invitations
		WHERE team_id = $1
		ORDER BY created_at DESC
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("query team invitations: %w", err)
	}
	defer rows.Close()

	var invitations []domain.TeamInvitation
	for rows.Next() {
		var invitation domain.TeamInvitation
		if err := rows.Scan(
			&invitation.ID, &invitation.TeamID, &invitation.Email, &invitation.Role, &invitation.Status,
			&invitation.ExpiresAt, &invitation.AcceptedAt, &invitation.AcceptedByUserID,
			&invitation.RevokedAt, &invitation.CreatedAt,
		); err != nil {
			return nil, err
		}
		invitations = append(invitations, invitation)
	}
	return invitations, rows.Err()
}

func (r *TeamRepo) RevokeInvitation(ctx context.Context, teamID, invitationID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE team_invitations
		SET status = 'revoked', revoked_at = NOW()
		WHERE id = $1 AND team_id = $2 AND status = 'pending'
	`, invitationID, teamID)
	return err
}

func (r *TeamRepo) AcceptInvitation(ctx context.Context, tokenHash string, userID uuid.UUID) (*domain.TeamInvitation, error) {
	var invitation domain.TeamInvitation
	err := r.db.QueryRow(ctx, `
		WITH accepted_invitation AS (
			UPDATE team_invitations AS i
			SET status = 'accepted', accepted_at = NOW(), accepted_by_user_id = $2
			FROM auth.users AS u
			WHERE i.token_hash = $1
				AND i.status = 'pending'
				AND i.expires_at > NOW()
				AND u.id = $2
				AND lower(i.email) = lower(u.email)
			RETURNING i.id, i.team_id, i.email, i.role, i.status, i.expires_at,
				i.accepted_at, i.accepted_by_user_id, i.revoked_at, i.created_at
		), member AS (
			INSERT INTO team_members (id, team_id, user_id, role)
			SELECT $3, team_id, $2, role FROM accepted_invitation
			ON CONFLICT (team_id, user_id) DO NOTHING
		)
		SELECT id, team_id, email, role, status, expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at
		FROM accepted_invitation
	`, tokenHash, userID, uuid.New()).Scan(
		&invitation.ID, &invitation.TeamID, &invitation.Email, &invitation.Role, &invitation.Status,
		&invitation.ExpiresAt, &invitation.AcceptedAt, &invitation.AcceptedByUserID,
		&invitation.RevokedAt, &invitation.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &invitation, nil
}
