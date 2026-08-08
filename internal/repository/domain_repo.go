package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

type DomainRepo struct {
	db *pgxpool.Pool
}

func NewDomainRepo(db *pgxpool.Pool) *DomainRepo {
	return &DomainRepo{db: db}
}

func (r *DomainRepo) Create(ctx context.Context, d *domain.Domain) error {
	dkimRecords, _ := json.Marshal(d.DKIMDNSRecords)
	err := r.db.QueryRow(ctx, `
		INSERT INTO domains (id, team_id, name, status, verification_token, verification_status, ses_verification_status, spf_status, mx_status, dkim_status, dmarc_status, dkim_dns_record, dkim_dns_records, spf_dns_record, mx_dns_record, dmarc_dns_record, verification_dns_record)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING created_at
	`, d.ID, d.TeamID, d.Name, d.Status, d.VerificationToken,
		d.VerificationStatus, d.SESVerificationStatus, d.SPFStatus, d.MXStatus, d.DKIMStatus, d.DMARCStatus,
		d.DKIMDNSRecord, dkimRecords, d.SPFDNSRecord, d.MXDNSRecord, d.DMARCDNSRecord, d.VerificationDNSRecord).Scan(&d.CreatedAt)
	return err
}

func (r *DomainRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Domain, error) {
	var d domain.Domain
	var dkimRecords []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, name, status, verification_token, verification_status, ses_verification_status, spf_status, mx_status, dkim_status, dmarc_status, dkim_dns_record, dkim_dns_records, spf_dns_record, mx_dns_record, dmarc_dns_record, verification_dns_record, created_at
		FROM domains WHERE id = $1
	`, id).Scan(
		&d.ID, &d.TeamID, &d.Name, &d.Status, &d.VerificationToken, &d.VerificationStatus, &d.SESVerificationStatus,
		&d.SPFStatus, &d.MXStatus, &d.DKIMStatus, &d.DMARCStatus,
		&d.DKIMDNSRecord, &dkimRecords, &d.SPFDNSRecord, &d.MXDNSRecord, &d.DMARCDNSRecord, &d.VerificationDNSRecord, &d.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(dkimRecords, &d.DKIMDNSRecords)
	return &d, nil
}

func (r *DomainRepo) GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*domain.Domain, error) {
	var d domain.Domain
	var dkimRecords []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, name, status, verification_token, verification_status, ses_verification_status, spf_status, mx_status, dkim_status, dmarc_status, dkim_dns_record, dkim_dns_records, spf_dns_record, mx_dns_record, dmarc_dns_record, verification_dns_record, created_at
		FROM domains WHERE id = $1 AND team_id = $2
	`, id, teamID).Scan(
		&d.ID, &d.TeamID, &d.Name, &d.Status, &d.VerificationToken, &d.VerificationStatus, &d.SESVerificationStatus,
		&d.SPFStatus, &d.MXStatus, &d.DKIMStatus, &d.DMARCStatus,
		&d.DKIMDNSRecord, &dkimRecords, &d.SPFDNSRecord, &d.MXDNSRecord, &d.DMARCDNSRecord, &d.VerificationDNSRecord, &d.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("domain not found")
	}
	_ = json.Unmarshal(dkimRecords, &d.DKIMDNSRecords)
	return &d, nil
}

func (r *DomainRepo) GetByName(ctx context.Context, teamID uuid.UUID, name string) (*domain.Domain, error) {
	var d domain.Domain
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, name, status, verification_token, spf_status, dkim_status, dmarc_status, created_at
		FROM domains WHERE team_id = $1 AND lower(trim(trailing '.' FROM name)) = lower(trim(trailing '.' FROM $2))
	`, teamID, name).Scan(
		&d.ID, &d.TeamID, &d.Name, &d.Status, &d.VerificationToken,
		&d.SPFStatus, &d.DKIMStatus, &d.DMARCStatus, &d.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DomainRepo) List(ctx context.Context, teamID uuid.UUID) (*domain.DomainListResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, name, status, verification_token, verification_status, ses_verification_status, spf_status, mx_status, dkim_status, dmarc_status, dkim_dns_records, created_at
		FROM domains WHERE team_id = $1
		ORDER BY created_at DESC
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []domain.Domain
	for rows.Next() {
		var d domain.Domain
		var dkimRecords []byte
		err := rows.Scan(&d.ID, &d.TeamID, &d.Name, &d.Status, &d.VerificationToken, &d.VerificationStatus,
			&d.SESVerificationStatus, &d.SPFStatus, &d.MXStatus, &d.DKIMStatus, &d.DMARCStatus, &dkimRecords, &d.CreatedAt)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(dkimRecords, &d.DKIMDNSRecords)
		domains = append(domains, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &domain.DomainListResponse{Data: domains}, nil
}

func (r *DomainRepo) Update(ctx context.Context, d *domain.Domain) error {
	dkimRecords, _ := json.Marshal(d.DKIMDNSRecords)
	_, err := r.db.Exec(ctx, `
		UPDATE domains SET status = $1, verification_status = $2, ses_verification_status = $3, spf_status = $4, mx_status = $5, dkim_status = $6, dmarc_status = $7,
			dkim_dns_record = $8, dkim_dns_records = $9, spf_dns_record = $10, mx_dns_record = $11, dmarc_dns_record = $12, verification_dns_record = $13
		WHERE id = $14
	`, d.Status, d.VerificationStatus, d.SESVerificationStatus, d.SPFStatus, d.MXStatus, d.DKIMStatus, d.DMARCStatus,
		d.DKIMDNSRecord, dkimRecords, d.SPFDNSRecord, d.MXDNSRecord, d.DMARCDNSRecord, d.VerificationDNSRecord, d.ID)
	return err
}

func (r *DomainRepo) UpdateForTeam(ctx context.Context, d *domain.Domain) error {
	dkimRecords, _ := json.Marshal(d.DKIMDNSRecords)
	_, err := r.db.Exec(ctx, `
		UPDATE domains SET status = $1, verification_status = $2, ses_verification_status = $3, spf_status = $4, mx_status = $5, dkim_status = $6, dmarc_status = $7,
			dkim_dns_record = $8, dkim_dns_records = $9, spf_dns_record = $10, mx_dns_record = $11, dmarc_dns_record = $12, verification_dns_record = $13
		WHERE id = $14 AND team_id = $15
	`, d.Status, d.VerificationStatus, d.SESVerificationStatus, d.SPFStatus, d.MXStatus, d.DKIMStatus, d.DMARCStatus,
		d.DKIMDNSRecord, dkimRecords, d.SPFDNSRecord, d.MXDNSRecord, d.DMARCDNSRecord, d.VerificationDNSRecord, d.ID, d.TeamID)
	return err
}

func (r *DomainRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM domains WHERE id = $1`, id)
	return err
}

func (r *DomainRepo) DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM domains WHERE id = $1 AND team_id = $2`, id, teamID)
	return err
}

func (r *DomainRepo) GetTeamByDomain(ctx context.Context, domainName string) (uuid.UUID, error) {
	var teamID uuid.UUID
	err := r.db.QueryRow(ctx, `
		SELECT team_id FROM domains
		WHERE lower(trim(trailing '.' FROM name)) = $1 AND status = 'verified' AND mx_status = 'verified'
	`, domainName).Scan(&teamID)
	return teamID, err
}
