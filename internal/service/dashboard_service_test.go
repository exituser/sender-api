package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type dashboardRepoStub struct {
	domain.DashboardRepository
	snapshot *domain.DashboardSnapshot
}

func (s *dashboardRepoStub) GetSnapshot(context.Context, uuid.UUID) (*domain.DashboardSnapshot, error) {
	return s.snapshot, nil
}

func TestDashboardSummaryExplainsMissingSenderSetup(t *testing.T) {
	service := NewDashboardService(&dashboardRepoStub{snapshot: &domain.DashboardSnapshot{}})
	summary, err := service.Summary(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Status != "action_needed" || len(summary.Alerts) != 1 || summary.Alerts[0].ActionHref != "/domains" {
		t.Fatalf("unexpected missing-domain summary: %+v", summary)
	}
	if summary.Alerts[0].Title != "Add a sender domain" {
		t.Fatalf("expected customer-facing title, got %q", summary.Alerts[0].Title)
	}
}

func TestDashboardSummarySeparatesMarketingRecommendationFromSendingBlock(t *testing.T) {
	service := NewDashboardService(&dashboardRepoStub{snapshot: &domain.DashboardSnapshot{
		Domains: []domain.DashboardDomainSnapshot{{
			Name:                  "example.com",
			Status:                "verified",
			SPFStatus:             "verified",
			DKIMStatus:            "verified",
			DMARCStatus:           "failed",
			SESVerificationStatus: "verified",
		}},
	}})
	summary, err := service.Summary(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Status != "attention" || len(summary.Alerts) != 1 || summary.Alerts[0].Code != "marketing_protection" {
		t.Fatalf("unexpected marketing summary: %+v", summary)
	}
	if !summary.Domains[0].Ready || summary.Domains[0].NextAction != "Improve sender protection" {
		t.Fatalf("expected domain to remain ready for transactional sending: %+v", summary.Domains[0])
	}
}

func TestDashboardSummaryReportsReadyStateWithoutTechnicalTerms(t *testing.T) {
	service := NewDashboardService(&dashboardRepoStub{snapshot: &domain.DashboardSnapshot{
		Domains: []domain.DashboardDomainSnapshot{{
			Name:                  "example.com",
			Status:                "verified",
			SPFStatus:             "verified",
			DKIMStatus:            "verified",
			DMARCStatus:           "verified",
			SESVerificationStatus: "verified",
		}},
	}})
	summary, err := service.Summary(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Status != "ready" || summary.StatusLabel != "Ready to send" || len(summary.Alerts) != 0 {
		t.Fatalf("unexpected ready summary: %+v", summary)
	}
}
