package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

type ContactRepo struct {
	db *pgxpool.Pool
}

func NewContactRepo(db *pgxpool.Pool) *ContactRepo {
	return &ContactRepo{db: db}
}

func (r *ContactRepo) Create(ctx context.Context, contact *domain.Contact) error {
	propsJSON, _ := json.Marshal(contact.Properties)
	subscribed := contact.Subscribed

	_, err := r.db.Exec(ctx, `
		INSERT INTO contacts (id, team_id, email, first_name, last_name, subscribed, properties)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, contact.ID, contact.TeamID, contact.Email, contact.FirstName, contact.LastName, subscribed, propsJSON)
	return err
}

func (r *ContactRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Contact, error) {
	var c domain.Contact
	var propsJSON []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, email, first_name, last_name, subscribed, properties, created_at, updated_at
		FROM contacts WHERE id = $1
	`, id).Scan(&c.ID, &c.TeamID, &c.Email, &c.FirstName, &c.LastName, &c.Subscribed, &propsJSON, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(propsJSON, &c.Properties)
	return &c, nil
}

func (r *ContactRepo) GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*domain.Contact, error) {
	contact, err := r.GetByID(ctx, id)
	if err != nil || contact.TeamID != teamID {
		return nil, fmt.Errorf("contact not found")
	}
	return contact, nil
}

func (r *ContactRepo) GetByEmail(ctx context.Context, teamID uuid.UUID, email string) (*domain.Contact, error) {
	var c domain.Contact
	var propsJSON []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, email, first_name, last_name, subscribed, properties, created_at, updated_at
		FROM contacts WHERE team_id = $1 AND email = $2
	`, teamID, email).Scan(&c.ID, &c.TeamID, &c.Email, &c.FirstName, &c.LastName, &c.Subscribed, &propsJSON, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(propsJSON, &c.Properties)
	return &c, nil
}

func (r *ContactRepo) List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*domain.ContactListResponse, error) {
	var total int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE team_id = $1`, teamID).Scan(&total)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, email, first_name, last_name, subscribed, created_at
		FROM contacts WHERE team_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []domain.Contact
	for rows.Next() {
		var c domain.Contact
		err := rows.Scan(&c.ID, &c.TeamID, &c.Email, &c.FirstName, &c.LastName, &c.Subscribed, &c.CreatedAt)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}

	return &domain.ContactListResponse{
		Data:   contacts,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (r *ContactRepo) Update(ctx context.Context, contact *domain.Contact) error {
	propsJSON, _ := json.Marshal(contact.Properties)
	_, err := r.db.Exec(ctx, `
		UPDATE contacts SET email = $1, first_name = $2, last_name = $3, subscribed = $4, properties = $5, updated_at = NOW()
		WHERE id = $6
	`, contact.Email, contact.FirstName, contact.LastName, contact.Subscribed, propsJSON, contact.ID)
	return err
}

func (r *ContactRepo) UpdateForTeam(ctx context.Context, contact *domain.Contact) error {
	propsJSON, _ := json.Marshal(contact.Properties)
	_, err := r.db.Exec(ctx, `
		UPDATE contacts SET email = $1, first_name = $2, last_name = $3, subscribed = $4, properties = $5, updated_at = NOW()
		WHERE id = $6 AND team_id = $7
	`, contact.Email, contact.FirstName, contact.LastName, contact.Subscribed, propsJSON, contact.ID, contact.TeamID)
	return err
}

func (r *ContactRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM contacts WHERE id = $1`, id)
	return err
}

func (r *ContactRepo) DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM contacts WHERE id = $1 AND team_id = $2`, id, teamID)
	return err
}

func (r *ContactRepo) BulkCreate(ctx context.Context, contacts []*domain.Contact) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, c := range contacts {
		propsJSON, _ := json.Marshal(c.Properties)
		if _, err := tx.Exec(ctx, `
			INSERT INTO contacts (id, team_id, email, first_name, last_name, subscribed, properties)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, c.ID, c.TeamID, c.Email, c.FirstName, c.LastName, c.Subscribed, propsJSON); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
