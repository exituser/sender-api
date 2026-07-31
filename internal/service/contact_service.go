package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
)

type ContactService struct {
	contactRepo domain.ContactRepository
	logger      *slog.Logger
}

func NewContactService(contactRepo domain.ContactRepository, logger *slog.Logger) *ContactService {
	return &ContactService{
		contactRepo: contactRepo,
		logger:      logger,
	}
}

func (s *ContactService) Create(ctx context.Context, teamID uuid.UUID, req *domain.CreateContactRequest) (*domain.Contact, error) {
	if req == nil {
		return nil, fmt.Errorf("contact request is required")
	}
	if !validator.IsValidEmail(req.Email) {
		return nil, fmt.Errorf("invalid contact email")
	}
	existing, _ := s.contactRepo.GetByEmail(ctx, teamID, req.Email)
	if existing != nil {
		return nil, fmt.Errorf("contact with email %s already exists", req.Email)
	}

	subscribed := true
	if req.Subscribed != nil {
		subscribed = *req.Subscribed
	}

	contact := &domain.Contact{
		ID:         uuid.New(),
		TeamID:     teamID,
		Email:      req.Email,
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		Subscribed: subscribed,
		Properties: req.Properties,
	}

	if err := s.contactRepo.Create(ctx, contact); err != nil {
		return nil, fmt.Errorf("failed to create contact: %w", err)
	}

	s.logger.Info("contact created", "contact_id", contact.ID)
	return contact, nil
}

func (s *ContactService) GetByID(ctx context.Context, teamID, id uuid.UUID) (*domain.Contact, error) {
	return s.contactRepo.GetByIDForTeam(ctx, teamID, id)
}

func (s *ContactService) List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*domain.ContactListResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return s.contactRepo.List(ctx, teamID, limit, offset)
}

func (s *ContactService) Update(ctx context.Context, teamID, id uuid.UUID, req *domain.UpdateContactRequest) (*domain.Contact, error) {
	if req == nil {
		return nil, fmt.Errorf("contact request is required")
	}
	contact, err := s.contactRepo.GetByIDForTeam(ctx, teamID, id)
	if err != nil {
		return nil, err
	}

	if req.Email != nil {
		if !validator.IsValidEmail(*req.Email) {
			return nil, fmt.Errorf("invalid contact email")
		}
		contact.Email = *req.Email
	}
	if req.FirstName != nil {
		contact.FirstName = req.FirstName
	}
	if req.LastName != nil {
		contact.LastName = req.LastName
	}
	if req.Subscribed != nil {
		contact.Subscribed = *req.Subscribed
	}
	if req.Properties != nil {
		contact.Properties = req.Properties
	}

	if err := s.contactRepo.UpdateForTeam(ctx, contact); err != nil {
		return nil, fmt.Errorf("failed to update contact: %w", err)
	}

	return contact, nil
}

func (s *ContactService) Delete(ctx context.Context, teamID, id uuid.UUID) error {
	return s.contactRepo.DeleteForTeam(ctx, teamID, id)
}

func (s *ContactService) ImportCSV(ctx context.Context, teamID uuid.UUID, contacts []*domain.CreateContactRequest) (int, error) {
	var created []*domain.Contact
	for _, req := range contacts {
		if req == nil {
			return 0, fmt.Errorf("contact request is required")
		}
		if !validator.IsValidEmail(req.Email) {
			return 0, fmt.Errorf("invalid contact email: %s", req.Email)
		}
		contact := &domain.Contact{
			ID:         uuid.New(),
			TeamID:     teamID,
			Email:      req.Email,
			FirstName:  req.FirstName,
			LastName:   req.LastName,
			Subscribed: true,
			Properties: req.Properties,
		}
		created = append(created, contact)
	}

	if err := s.contactRepo.BulkCreate(ctx, created); err != nil {
		return 0, fmt.Errorf("failed to import contacts: %w", err)
	}

	s.logger.Info("contacts imported", "team_id", teamID, "count", len(created))
	return len(created), nil
}
