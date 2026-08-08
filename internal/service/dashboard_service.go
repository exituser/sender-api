package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type DashboardService struct {
	repo                  domain.DashboardRepository
	deliveryEventsEnabled bool
}

func NewDashboardService(repo domain.DashboardRepository, deliveryEventsEnabled ...bool) *DashboardService {
	enabled := true
	if len(deliveryEventsEnabled) > 0 {
		enabled = deliveryEventsEnabled[0]
	}
	return &DashboardService{repo: repo, deliveryEventsEnabled: enabled}
}

func (s *DashboardService) Summary(ctx context.Context, teamID uuid.UUID) (*domain.DashboardSummary, error) {
	snapshot, err := s.repo.GetSnapshot(ctx, teamID)
	if err != nil {
		return nil, err
	}

	summary := &domain.DashboardSummary{
		Status:      "ready",
		StatusLabel: "Ready to send",
		Delivery:    snapshot.Delivery,
		Tracking: domain.DashboardTracking{
			Configured: s.deliveryEventsEnabled,
			Label:      "Delivery updates are off",
		},
		Audience: snapshot.Audience,
		Webhooks: snapshot.Webhooks,
		Activity: snapshot.Activity,
		Alerts:   make([]domain.DashboardAlert, 0),
		Domains:  make([]domain.DashboardDomain, 0, len(snapshot.Domains)),
	}
	if s.deliveryEventsEnabled {
		summary.Tracking.Label = "Delivery updates are on"
	} else {
		summary.Alerts = append(summary.Alerts, domain.DashboardAlert{
			Code:        "delivery_tracking_disabled",
			Severity:    domain.DashboardAlertWarning,
			Title:       "Delivery updates are not connected",
			Description: "You can see when a message is accepted, but delivery, bounce, and complaint updates are not connected yet.",
			ActionLabel: "Read setup guide",
			ActionHref:  "/docs",
		})
	}

	if len(snapshot.Domains) == 0 {
		summary.Alerts = append(summary.Alerts, domain.DashboardAlert{
			Code:        "no_sender_domain",
			Severity:    domain.DashboardAlertCritical,
			Title:       "Add a sender domain",
			Description: "Connect the domain you want to send from before sending your first email.",
			ActionLabel: "Add a domain",
			ActionHref:  "/domains",
		})
	} else {
		for _, item := range snapshot.Domains {
			domainSummary := domain.DashboardDomain{Name: item.Name, Ready: item.Status == "verified"}
			if item.Status != "verified" {
				domainSummary.NextAction = "Finish sender setup"
				domainSummary.Issues = append(domainSummary.Issues, "Publish the requested records and verify the domain")
				summary.Alerts = append(summary.Alerts, domain.DashboardAlert{
					Code:        "sender_domain_setup",
					Severity:    domain.DashboardAlertCritical,
					Title:       "Finish setup for " + item.Name,
					Description: item.Name + " is not ready to send yet. Publish the records shown in Domains, then verify it.",
					ActionLabel: "Finish setup",
					ActionHref:  "/domains",
				})
			}
			if item.SPFStatus != "verified" {
				domainSummary.Issues = append(domainSummary.Issues, "Complete the sender records")
			}
			if item.DKIMStatus != "verified" {
				domainSummary.Issues = append(domainSummary.Issues, "Complete email authentication")
			}
			if item.DMARCStatus != "verified" {
				domainSummary.Issues = append(domainSummary.Issues, "Add a DMARC policy to enable marketing sending")
				if item.Status == "verified" {
					domainSummary.NextAction = "Improve sender protection"
					summary.Alerts = append(summary.Alerts, domain.DashboardAlert{
						Code:        "marketing_protection",
						Severity:    domain.DashboardAlertWarning,
						Title:       "Improve protection for " + item.Name,
						Description: "Add sender protection for " + item.Name + " to unlock marketing messages.",
						ActionLabel: "Improve protection",
						ActionHref:  "/domains",
					})
				}
			}
			if item.SESVerificationStatus != "verified" {
				domainSummary.Issues = append(domainSummary.Issues, "Complete provider verification")
			}
			if domainSummary.Ready && domainSummary.NextAction == "" {
				domainSummary.NextAction = "All essential checks passed"
			}
			summary.Domains = append(summary.Domains, domainSummary)
		}
	}

	if snapshot.Delivery.Failed > 0 {
		summary.Alerts = append(summary.Alerts, domain.DashboardAlert{
			Code:        "failed_messages",
			Severity:    domain.DashboardAlertWarning,
			Title:       "Some messages need attention",
			Description: "Some recent messages could not be sent. Review the email activity to see what happened.",
			ActionLabel: "Review email activity",
			ActionHref:  "/emails",
		})
	}
	if snapshot.Webhooks.Failed > 0 {
		summary.Alerts = append(summary.Alerts, domain.DashboardAlert{
			Code:        "webhook_delivery",
			Severity:    domain.DashboardAlertWarning,
			Title:       "Delivery updates need attention",
			Description: "Some status updates could not reach your app. Review the connection so delivery events stay current.",
			ActionLabel: "Review connections",
			ActionHref:  "/webhooks",
		})
	}

	for _, alert := range summary.Alerts {
		if alert.Severity == domain.DashboardAlertCritical {
			summary.Status = "action_needed"
			summary.StatusLabel = "Action needed"
			break
		}
		if alert.Severity == domain.DashboardAlertWarning {
			summary.Status = "attention"
			summary.StatusLabel = "Needs attention"
		}
	}
	return summary, nil
}
