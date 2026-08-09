package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sender-api/sender-api/internal/billing"
	"github.com/sender-api/sender-api/internal/domain"
)

var ErrBillingNotConfigured = errors.New("billing is not configured")

type BillingStore interface {
	SetStripeCustomerID(ctx context.Context, teamID uuid.UUID, customerID string) error
	GetByStripeCustomerID(ctx context.Context, customerID string) (*domain.Team, error)
	GetByStripeSubscriptionID(ctx context.Context, subscriptionID string) (*domain.Team, error)
	UpdateBilling(ctx context.Context, teamID uuid.UUID, customerID, subscriptionID *string, plan domain.Plan, status string, currentPeriodEnd *time.Time, cancelAtPeriodEnd bool) error
	ApplyBillingEvent(ctx context.Context, eventID string, eventCreated int64, eventType string, teamID uuid.UUID, customerID, subscriptionID *string, plan domain.Plan, status string, currentPeriodEnd *time.Time, cancelAtPeriodEnd bool) error
}

type BillingService struct {
	teamRepo domain.TeamRepository
	store    BillingStore
	stripe   *billing.StripeClient
	logger   *slog.Logger
}

func NewBillingService(teamRepo domain.TeamRepository, store BillingStore, stripe *billing.StripeClient, logger *slog.Logger) *BillingService {
	if logger == nil {
		logger = slog.Default()
	}
	return &BillingService{teamRepo: teamRepo, store: store, stripe: stripe, logger: logger}
}

func (s *BillingService) Summary(ctx context.Context, teamID uuid.UUID) (*domain.BillingSummary, error) {
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return &domain.BillingSummary{
		Plan:              team.Plan,
		Status:            billingStatus(team),
		CurrentPeriodEnd:  team.CurrentPeriodEnd,
		CancelAtPeriodEnd: team.CancelAtPeriodEnd,
		HasCustomer:       team.StripeCustomerID != nil && strings.TrimSpace(*team.StripeCustomerID) != "",
		HasSubscription:   team.StripeSubscriptionID != nil && strings.TrimSpace(*team.StripeSubscriptionID) != "",
		Configured:        s.stripe != nil && s.stripe.Configured(),
	}, nil
}

func (s *BillingService) Checkout(ctx context.Context, teamID uuid.UUID, requestedPlan domain.Plan) (*domain.BillingSessionResponse, error) {
	if s.stripe == nil || !s.stripe.Configured() {
		return nil, ErrBillingNotConfigured
	}
	if s.store == nil || s.teamRepo == nil {
		return nil, errors.New("billing storage is not configured")
	}
	plan := normalizePlan(requestedPlan)
	if plan == domain.PlanFree {
		return nil, errors.New("free plan does not require checkout")
	}
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	customerID := ""
	if team.StripeCustomerID != nil {
		customerID = strings.TrimSpace(*team.StripeCustomerID)
	}
	if customerID == "" {
		customerID, err = s.stripe.CreateCustomer(ctx, team)
		if err != nil {
			return nil, err
		}
		if err := s.store.SetStripeCustomerID(ctx, teamID, customerID); err != nil {
			return nil, fmt.Errorf("save Stripe customer: %w", err)
		}
	}
	session, err := s.stripe.CreateCheckoutSession(ctx, teamID, customerID, plan)
	if err != nil {
		return nil, err
	}
	return &domain.BillingSessionResponse{ID: session.ID, URL: session.URL}, nil
}

func (s *BillingService) Portal(ctx context.Context, teamID uuid.UUID) (*domain.BillingSessionResponse, error) {
	if s.stripe == nil || !s.stripe.Configured() {
		return nil, ErrBillingNotConfigured
	}
	if s.store == nil || s.teamRepo == nil {
		return nil, errors.New("billing storage is not configured")
	}
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, err
	}
	if team.StripeCustomerID == nil || strings.TrimSpace(*team.StripeCustomerID) == "" {
		return nil, errors.New("team has no billing customer")
	}
	session, err := s.stripe.CreatePortalSession(ctx, strings.TrimSpace(*team.StripeCustomerID))
	if err != nil {
		return nil, err
	}
	return &domain.BillingSessionResponse{ID: session.ID, URL: session.URL}, nil
}

func (s *BillingService) HandleWebhook(ctx context.Context, payload []byte, signature string, now time.Time) error {
	if s.stripe == nil || !s.stripe.WebhookConfigured() {
		return ErrBillingNotConfigured
	}
	event, err := s.stripe.VerifyWebhook(payload, signature, now)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(event.Data.Object, &object); err != nil {
		return fmt.Errorf("decode Stripe event object: %w", err)
	}
	return s.applyWebhookEvent(ctx, event.ID, event.Created, event.Type, object)
}

func (s *BillingService) applyWebhookEvent(ctx context.Context, eventID string, eventCreated int64, eventType string, object map[string]json.RawMessage) error {
	metadata := stringMap(objectValue(object, "metadata"))
	customerID := stringValue(object, "customer")
	subscriptionID := stringValue(object, "subscription")
	if subscriptionID == "" {
		subscriptionID = stringValue(object, "id")
	}
	team, err := s.resolveTeam(ctx, metadata["team_id"], customerID, subscriptionID)
	if err != nil {
		return err
	}
	if team == nil {
		if s.logger != nil {
			s.logger.Warn("ignoring Stripe event for unknown team", "event", eventType, "customer_id", customerID, "subscription_id", subscriptionID)
		}
		return nil
	}
	if customerID == "" && team.StripeCustomerID != nil {
		customerID = *team.StripeCustomerID
	}
	var customerPtr, subscriptionPtr *string
	if customerID != "" {
		customerPtr = &customerID
	}
	if subscriptionID != "" {
		subscriptionPtr = &subscriptionID
	}

	switch eventType {
	case "checkout.session.completed":
		plan := planFromMetadata(metadata, domain.PlanFree)
		if subscriptionID == "" {
			return s.applyBillingEvent(ctx, eventID, eventCreated, eventType, team.ID, customerPtr, nil, plan, "active", nil, false)
		}
		return s.applyBillingEvent(ctx, eventID, eventCreated, eventType, team.ID, customerPtr, subscriptionPtr, plan, "active", nil, false)
	case "customer.subscription.created", "customer.subscription.updated":
		status := stringValue(object, "status")
		if status == "" {
			status = "active"
		}
		plan, _ := planFromPrice(object, s.stripe)
		if !subscriptionIsActive(status) && plan != domain.PlanFree {
			plan = domain.PlanFree
		}
		periodEnd := timeValue(object, "current_period_end")
		cancelAtPeriodEnd := boolValue(object, "cancel_at_period_end")
		return s.applyBillingEvent(ctx, eventID, eventCreated, eventType, team.ID, customerPtr, subscriptionPtr, plan, status, periodEnd, cancelAtPeriodEnd)
	case "customer.subscription.deleted":
		status := stringValue(object, "status")
		if status == "" {
			status = "canceled"
		}
		return s.applyBillingEvent(ctx, eventID, eventCreated, eventType, team.ID, customerPtr, nil, domain.PlanFree, status, nil, false)
	case "invoice.payment_failed":
		return s.applyBillingEvent(ctx, eventID, eventCreated, eventType, team.ID, customerPtr, subscriptionPtr, domain.PlanFree, "past_due", nil, false)
	default:
		return nil
	}
}

func (s *BillingService) applyBillingEvent(ctx context.Context, eventID string, eventCreated int64, eventType string, teamID uuid.UUID, customerID, subscriptionID *string, plan domain.Plan, status string, currentPeriodEnd *time.Time, cancelAtPeriodEnd bool) error {
	return s.store.ApplyBillingEvent(ctx, eventID, eventCreated, eventType, teamID, customerID, subscriptionID, plan, status, currentPeriodEnd, cancelAtPeriodEnd)
}

func (s *BillingService) resolveTeam(ctx context.Context, metadataTeamID, customerID, subscriptionID string) (*domain.Team, error) {
	if id, err := uuid.Parse(strings.TrimSpace(metadataTeamID)); err == nil && id != uuid.Nil {
		team, err := s.teamRepo.GetByID(ctx, id)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			return team, nil
		}
	}
	if customerID != "" {
		if team, err := s.store.GetByStripeCustomerID(ctx, customerID); err == nil {
			return team, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if subscriptionID != "" {
		if team, err := s.store.GetByStripeSubscriptionID(ctx, subscriptionID); err == nil {
			return team, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return nil, nil
}

func normalizePlan(plan domain.Plan) domain.Plan {
	switch domain.Plan(strings.ToLower(strings.TrimSpace(string(plan)))) {
	case domain.PlanPro:
		return domain.PlanPro
	case domain.PlanScale:
		return domain.PlanScale
	default:
		return domain.PlanFree
	}
}

func planFromMetadata(metadata map[string]string, fallback domain.Plan) domain.Plan {
	if plan := normalizePlan(domain.Plan(metadata["plan"])); plan != domain.PlanFree || metadata["plan"] == string(domain.PlanFree) {
		return plan
	}
	return normalizePlan(fallback)
}

func planFromPrice(object map[string]json.RawMessage, stripe *billing.StripeClient) (domain.Plan, bool) {
	items, ok := object["items"]
	if !ok || stripe == nil {
		return domain.PlanFree, false
	}
	var value struct {
		Data []struct {
			Price struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	}
	if json.Unmarshal(items, &value) != nil || len(value.Data) == 0 {
		return domain.PlanFree, false
	}
	return stripe.PlanForPrice(value.Data[0].Price.ID)
}

func billingStatus(team *domain.Team) string {
	if team == nil || strings.TrimSpace(team.BillingStatus) == "" {
		return "inactive"
	}
	return team.BillingStatus
}

func subscriptionIsActive(status string) bool {
	return status == "active" || status == "trialing"
}

func objectValue(object map[string]json.RawMessage, key string) json.RawMessage { return object[key] }

func stringValue(object map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(object[key], &value)
	return strings.TrimSpace(value)
}

func stringMap(raw json.RawMessage) map[string]string {
	var values map[string]string
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return map[string]string{}
	}
	return values
}

func timeValue(object map[string]json.RawMessage, key string) *time.Time {
	var seconds int64
	if json.Unmarshal(object[key], &seconds) != nil || seconds <= 0 {
		return nil
	}
	value := time.Unix(seconds, 0).UTC()
	return &value
}

func boolValue(object map[string]json.RawMessage, key string) bool {
	var value bool
	_ = json.Unmarshal(object[key], &value)
	return value
}
