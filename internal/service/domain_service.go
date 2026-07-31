package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
)

type DomainService struct {
	domainRepo domain.DomainRepository
	logger     *slog.Logger
	awsRegion  string
}

func NewDomainService(domainRepo domain.DomainRepository, logger *slog.Logger, awsRegion string) *DomainService {
	return &DomainService{
		domainRepo: domainRepo,
		logger:     logger,
		awsRegion:  strings.ToLower(strings.TrimSpace(awsRegion)),
	}
}

func (s *DomainService) Create(ctx context.Context, teamID uuid.UUID, req *domain.CreateDomainRequest) (*domain.DomainResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("domain request is required")
	}
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(req.Name), "."))
	if !validator.IsValidDomain(name) {
		return nil, fmt.Errorf("invalid domain name")
	}
	existing, _ := s.domainRepo.GetByName(ctx, teamID, name)
	if existing != nil {
		return nil, fmt.Errorf("domain %s already exists", name)
	}

	token, err := s.generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate verification token: %w", err)
	}

	d := &domain.Domain{
		ID:                 uuid.New(),
		TeamID:             teamID,
		Name:               name,
		Status:             domain.DomainStatusPending,
		VerificationToken:  token,
		VerificationStatus: "pending",
		SPFStatus:          "pending",
		MXStatus:           "pending",
		DKIMStatus:         "not_configured",
		DMARCStatus:        "pending",
	}

	spfRecord := "v=spf1 include:amazonses.com ~all"
	d.MXDNSRecord = stringPtr(inboundMXHost(s.awsRegion))
	d.SPFDNSRecord = &spfRecord

	if err := s.domainRepo.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("failed to create domain: %w", err)
	}

	verificationHost := "_sender-api-verification." + name
	d.VerificationDNSRecord = &verificationHost

	if err := s.domainRepo.Update(ctx, d); err != nil {
		_ = s.domainRepo.Delete(ctx, d.ID)
		return nil, fmt.Errorf("failed to save domain DNS records: %w", err)
	}

	s.logger.Info("domain created", "domain_id", d.ID, "name", d.Name)

	return &domain.DomainResponse{
		ID:     d.ID,
		Name:   d.Name,
		Status: d.Status,
		DNSRecords: []domain.DNSRecord{
			{Type: domain.DNSRecordTypeTXT, Host: "@", Value: *d.SPFDNSRecord, TTL: 3600, Status: d.SPFStatus},
			{Type: domain.DNSRecordTypeMX, Host: "@", Value: *d.MXDNSRecord, TTL: 3600, Status: d.MXStatus},
			{Type: domain.DNSRecordTypeTXT, Host: "_sender-api-verification", Value: token, TTL: 3600, Status: d.VerificationStatus},
		},
		Instructions: "Add the SPF, SES inbound MX, and Sender API verification TXT records, then call the verify endpoint.",
		CreatedAt:    d.CreatedAt,
	}, nil
}

func (s *DomainService) GetByID(ctx context.Context, teamID, id uuid.UUID) (*domain.Domain, error) {
	return s.domainRepo.GetByIDForTeam(ctx, teamID, id)
}

func (s *DomainService) List(ctx context.Context, teamID uuid.UUID) (*domain.DomainListResponse, error) {
	return s.domainRepo.List(ctx, teamID)
}

func (s *DomainService) Verify(ctx context.Context, teamID, domainID uuid.UUID) error {
	d, err := s.domainRepo.GetByIDForTeam(ctx, teamID, domainID)
	if err != nil {
		return err
	}
	d.SPFStatus = "pending"
	d.MXStatus = "pending"
	d.VerificationStatus = "pending"
	d.Status = domain.DomainStatusPending
	if d.MXDNSRecord == nil {
		d.MXDNSRecord = stringPtr(inboundMXHost(s.awsRegion))
	}

	txtRecords, err := net.LookupTXT(d.Name)
	if err == nil {
		for _, record := range txtRecords {
			if containsSPF(record) {
				d.SPFStatus = "verified"
				break
			}
		}
	}

	if d.VerificationDNSRecord != nil && d.VerificationToken != "" {
		verificationRecords, lookupErr := net.LookupTXT(*d.VerificationDNSRecord)
		if lookupErr == nil {
			for _, record := range verificationRecords {
				if record == d.VerificationToken {
					d.VerificationStatus = "verified"
					break
				}
			}
		}
	}

	mxRecords, mxErr := net.LookupMX(d.Name)
	if mxErr == nil {
		if containsInboundMX(mxRecords, s.awsRegion) {
			d.MXStatus = "verified"
		} else {
			d.MXStatus = "failed"
		}
	}

	if d.SPFStatus == "verified" && d.VerificationStatus == "verified" && d.MXStatus == "verified" {
		d.Status = domain.DomainStatusVerified
	}

	return s.domainRepo.UpdateForTeam(ctx, d)
}

func (s *DomainService) Delete(ctx context.Context, teamID, id uuid.UUID) error {
	return s.domainRepo.DeleteForTeam(ctx, teamID, id)
}

func inboundMXHost(region string) string {
	region = strings.ToLower(strings.TrimSpace(region))
	if strings.HasPrefix(region, "cn-") {
		return "inbound-smtp." + region + ".amazonaws.com.cn"
	}
	return "inbound-smtp." + region + ".amazonaws.com"
}

func containsInboundMX(records []*net.MX, region string) bool {
	expected := strings.TrimSuffix(strings.ToLower(inboundMXHost(region)), ".")
	if expected == "inbound-smtp..amazonaws.com" || expected == "inbound-smtp..amazonaws.com.cn" {
		return false
	}
	for _, record := range records {
		if record == nil {
			continue
		}
		if strings.TrimSuffix(strings.ToLower(strings.TrimSpace(record.Host)), ".") == expected {
			return true
		}
	}
	return false
}

func stringPtr(value string) *string {
	return &value
}

func (s *DomainService) generateToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "sender-api-verify-" + hex.EncodeToString(bytes), nil
}

func containsSPF(record string) bool {
	return len(record) >= 4 && (strings.EqualFold(record[:4], "v=spf")) && strings.Contains(strings.ToLower(record), "include:amazonses.com")
}
