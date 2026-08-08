package domain

import (
	"time"

	"github.com/google/uuid"
)

type DashboardAlertSeverity string

const (
	DashboardAlertCritical DashboardAlertSeverity = "critical"
	DashboardAlertWarning  DashboardAlertSeverity = "warning"
	DashboardAlertInfo     DashboardAlertSeverity = "info"
)

type DashboardAlert struct {
	Code        string                 `json:"code"`
	Severity    DashboardAlertSeverity `json:"severity"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	ActionLabel string                 `json:"action_label"`
	ActionHref  string                 `json:"action_href"`
}

type DashboardDomain struct {
	Name       string   `json:"name"`
	Ready      bool     `json:"ready"`
	NextAction string   `json:"next_action,omitempty"`
	Issues     []string `json:"issues,omitempty"`
}

type DashboardDelivery struct {
	PeriodDays int64 `json:"period_days"`
	Total      int64 `json:"total"`
	Accepted   int64 `json:"accepted"`
	Delivered  int64 `json:"delivered"`
	Bounced    int64 `json:"bounced"`
	Complained int64 `json:"complained"`
	Failed     int64 `json:"failed"`
	Queued     int64 `json:"queued"`
}

type DashboardTracking struct {
	Configured bool   `json:"configured"`
	Label      string `json:"label"`
}

type DashboardAudience struct {
	UnsubscribedContacts int64 `json:"unsubscribed_contacts"`
	Suppressed           int64 `json:"suppressed"`
	Unsubscribed         int64 `json:"unsubscribed"`
	Bounced              int64 `json:"bounced"`
	Complained           int64 `json:"complained"`
}

type DashboardWebhooks struct {
	Configured int64 `json:"configured"`
	Failed     int64 `json:"failed"`
	Pending    int64 `json:"pending"`
}

type DashboardActivity struct {
	ID        uuid.UUID `json:"id"`
	EmailID   uuid.UUID `json:"email_id"`
	Event     string    `json:"event"`
	Subject   string    `json:"subject"`
	Timestamp time.Time `json:"timestamp"`
}

type DashboardSummary struct {
	Status      string              `json:"status"`
	StatusLabel string              `json:"status_label"`
	Alerts      []DashboardAlert    `json:"alerts"`
	Domains     []DashboardDomain   `json:"domains"`
	Delivery    DashboardDelivery   `json:"delivery"`
	Tracking    DashboardTracking   `json:"delivery_tracking"`
	Audience    DashboardAudience   `json:"audience"`
	Webhooks    DashboardWebhooks   `json:"webhooks"`
	Activity    []DashboardActivity `json:"activity"`
}

type DashboardDomainSnapshot struct {
	Name                  string
	Status                string
	SPFStatus             string
	DKIMStatus            string
	DMARCStatus           string
	SESVerificationStatus string
}

type DashboardSnapshot struct {
	Domains  []DashboardDomainSnapshot
	Delivery DashboardDelivery
	Audience DashboardAudience
	Webhooks DashboardWebhooks
	Activity []DashboardActivity
}
