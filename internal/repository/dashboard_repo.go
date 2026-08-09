package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

type DashboardRepo struct {
	db *pgxpool.Pool
}

func NewDashboardRepo(db *pgxpool.Pool) *DashboardRepo {
	return &DashboardRepo{db: db}
}

func (r *DashboardRepo) GetSnapshot(ctx context.Context, teamID uuid.UUID) (*domain.DashboardSnapshot, error) {
	snapshot := &domain.DashboardSnapshot{}

	rows, err := r.db.Query(ctx, `
		SELECT name, status, spf_status, dkim_status, dmarc_status, ses_verification_status
		FROM domains
		WHERE team_id = $1
		ORDER BY created_at DESC
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("load dashboard domains: %w", err)
	}
	for rows.Next() {
		var item domain.DashboardDomainSnapshot
		if err := rows.Scan(&item.Name, &item.Status, &item.SPFStatus, &item.DKIMStatus, &item.DMARCStatus, &item.SESVerificationStatus); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan dashboard domain: %w", err)
		}
		snapshot.Domains = append(snapshot.Domains, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate dashboard domains: %w", err)
	}
	rows.Close()

	if err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*)::bigint,
			COUNT(*) FILTER (WHERE status IN ('sent', 'delivered', 'opened', 'clicked'))::bigint,
			COUNT(*) FILTER (WHERE status IN ('delivered', 'opened', 'clicked'))::bigint,
			COUNT(*) FILTER (WHERE status = 'bounced')::bigint,
			COUNT(*) FILTER (WHERE status = 'complained')::bigint,
			COUNT(*) FILTER (WHERE status = 'failed')::bigint,
			COUNT(*) FILTER (WHERE status = 'ambiguous')::bigint,
			COUNT(*) FILTER (WHERE status IN ('queued', 'sending'))::bigint
		FROM emails
		WHERE team_id = $1 AND created_at >= NOW() - INTERVAL '7 days'
	`, teamID).Scan(
		&snapshot.Delivery.Total,
		&snapshot.Delivery.Accepted,
		&snapshot.Delivery.Delivered,
		&snapshot.Delivery.Bounced,
		&snapshot.Delivery.Complained,
		&snapshot.Delivery.Failed,
		&snapshot.Delivery.Uncertain,
		&snapshot.Delivery.Queued,
	); err != nil {
		return nil, fmt.Errorf("load dashboard delivery stats: %w", err)
	}
	snapshot.Delivery.PeriodDays = 7

	if err := r.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM contacts WHERE team_id = $1 AND subscribed = false),
			COUNT(*)::bigint,
			COUNT(*) FILTER (WHERE reason = 'unsubscribe')::bigint,
			COUNT(*) FILTER (WHERE reason = 'bounce')::bigint,
			COUNT(*) FILTER (WHERE reason = 'complaint')::bigint
		FROM suppressions
		WHERE team_id = $1
	`, teamID).Scan(
		&snapshot.Audience.UnsubscribedContacts,
		&snapshot.Audience.Suppressed,
		&snapshot.Audience.Unsubscribed,
		&snapshot.Audience.Bounced,
		&snapshot.Audience.Complained,
	); err != nil {
		return nil, fmt.Errorf("load dashboard audience stats: %w", err)
	}

	if err := r.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM webhooks WHERE team_id = $1 AND active = true),
			COUNT(*) FILTER (WHERE d.status = 'failed')::bigint,
			COUNT(*) FILTER (WHERE d.status IN ('pending', 'sending'))::bigint
		FROM webhook_deliveries d
		JOIN webhooks w ON w.id = d.webhook_id
		WHERE w.team_id = $1
	`, teamID).Scan(&snapshot.Webhooks.Configured, &snapshot.Webhooks.Failed, &snapshot.Webhooks.Pending); err != nil {
		return nil, fmt.Errorf("load dashboard webhook stats: %w", err)
	}

	activityRows, err := r.db.Query(ctx, `
		SELECT ev.id, ev.email_id, ev.event, e.subject, ev.timestamp
		FROM email_events ev
		JOIN emails e ON e.id = ev.email_id
		WHERE e.team_id = $1
		ORDER BY ev.timestamp DESC
		LIMIT 8
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("load dashboard activity: %w", err)
	}
	for activityRows.Next() {
		var item domain.DashboardActivity
		if err := activityRows.Scan(&item.ID, &item.EmailID, &item.Event, &item.Subject, &item.Timestamp); err != nil {
			activityRows.Close()
			return nil, fmt.Errorf("scan dashboard activity: %w", err)
		}
		snapshot.Activity = append(snapshot.Activity, item)
	}
	if err := activityRows.Err(); err != nil {
		activityRows.Close()
		return nil, fmt.Errorf("iterate dashboard activity: %w", err)
	}
	activityRows.Close()

	return snapshot, nil
}
