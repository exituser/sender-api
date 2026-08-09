package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sender-api/sender-api/internal/domain"
)

type resolveTeamRepoStub struct {
	domain.TeamRepository
	team *domain.Team
	err  error
}

func (s *resolveTeamRepoStub) GetByID(context.Context, uuid.UUID) (*domain.Team, error) {
	return s.team, s.err
}

type resolveTeamStoreStub struct {
	BillingStore
	customerTeam, subscriptionTeam *domain.Team
	customerErr, subscriptionErr   error
}

func (s *resolveTeamStoreStub) GetByStripeCustomerID(context.Context, string) (*domain.Team, error) {
	return s.customerTeam, s.customerErr
}

func (s *resolveTeamStoreStub) GetByStripeSubscriptionID(context.Context, string) (*domain.Team, error) {
	return s.subscriptionTeam, s.subscriptionErr
}

func TestBillingResolveTeamPropagatesDatabaseErrors(t *testing.T) {
	want := errors.New("database unavailable")
	service := &BillingService{
		teamRepo: &resolveTeamRepoStub{err: want},
		store:    &resolveTeamStoreStub{},
	}
	_, err := service.resolveTeam(context.Background(), uuid.New().String(), "customer", "subscription")
	if !errors.Is(err, want) {
		t.Fatalf("resolveTeam() error = %v, want %v", err, want)
	}
}

func TestBillingResolveTeamFallsBackAfterNotFound(t *testing.T) {
	want := &domain.Team{ID: uuid.New()}
	service := &BillingService{
		teamRepo: &resolveTeamRepoStub{err: pgx.ErrNoRows},
		store: &resolveTeamStoreStub{
			customerErr:      pgx.ErrNoRows,
			subscriptionTeam: want,
			subscriptionErr:  nil,
		},
	}
	got, err := service.resolveTeam(context.Background(), uuid.New().String(), "customer", "subscription")
	if err != nil {
		t.Fatalf("resolveTeam() error = %v", err)
	}
	if got != want {
		t.Fatalf("resolveTeam() = %#v, want %#v", got, want)
	}
}
