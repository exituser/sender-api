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
	domainRepo  domain.DomainRepository
	logger      *slog.Logger
	awsRegion   string
	sesIdentity domain.SESIdentityProvider
}

func NewDomainService(domainRepo domain.DomainRepository, logger *slog.Logger, awsRegion string, identityProviders ...domain.SESIdentityProvider) *DomainService {
	if logger == nil {
		logger = slog.Default()
	}
	service := &DomainService{
		domainRepo: domainRepo,
		logger:     logger,
		awsRegion:  strings.ToLower(strings.TrimSpace(awsRegion)),
	}
	if len(identityProviders) > 0 {
		service.sesIdentity = identityProviders[0]
	}
	return service
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
		ID:                    uuid.New(),
		TeamID:                teamID,
		Name:                  name,
		Status:                domain.DomainStatusPending,
		VerificationToken:     token,
		VerificationStatus:    "pending",
		SESVerificationStatus: "pending",
		SPFStatus:             "pending",
		MXStatus:              "pending",
		DKIMStatus:            "pending",
		DMARCStatus:           "pending",
	}

	spfRecord := "v=spf1 include:amazonses.com ~all"
	d.MXDNSRecord = stringPtr(inboundMXHost(s.awsRegion))
	d.SPFDNSRecord = &spfRecord
	verificationHost := "_sender-api-verification." + name
	d.VerificationDNSRecord = &verificationHost
	d.DMARCDNSRecord = stringPtr("_dmarc." + name)

	if s.sesIdentity != nil {
		identity, identityErr := s.sesIdentity.Create(ctx, name)
		if identityErr != nil {
			return nil, fmt.Errorf("failed to initialize SES identity: %w", identityErr)
		}
		s.applySESIdentity(d, identity)
	}

	if err := s.domainRepo.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("failed to create domain: %w", err)
	}

	s.logger.Info("domain created", "domain_id", d.ID, "name", d.Name)

	return &domain.DomainResponse{
		ID:     d.ID,
		Name:   d.Name,
		Status: d.Status,
		DNSRecords: append([]domain.DNSRecord{
			{Type: domain.DNSRecordTypeTXT, Host: "@", Value: *d.SPFDNSRecord, TTL: 3600, Status: d.SPFStatus},
			{Type: domain.DNSRecordTypeMX, Host: "@", Value: *d.MXDNSRecord, TTL: 3600, Status: d.MXStatus, Optional: true},
			{Type: domain.DNSRecordTypeTXT, Host: "_sender-api-verification", Value: token, TTL: 3600, Status: d.VerificationStatus},
			{Type: domain.DNSRecordTypeTXT, Host: "_dmarc", Value: "v=DMARC1; p=none", TTL: 3600, Status: d.DMARCStatus},
		}, d.DKIMDNSRecords...),
		Instructions: "Add or merge the SPF, Sender API verification TXT, DKIM CNAME, and DMARC TXT records, then call the verify endpoint. The SES inbound MX record is optional and is only needed if you enable inbound email. Never replace existing MX records without reviewing your current mail routing.",
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
	d.SESVerificationStatus = "pending"
	d.Status = domain.DomainStatusPending
	if d.MXDNSRecord == nil {
		d.MXDNSRecord = stringPtr(inboundMXHost(s.awsRegion))
	}
	if d.DMARCDNSRecord == nil {
		d.DMARCDNSRecord = stringPtr("_dmarc." + d.Name)
	}

	if s.sesIdentity != nil {
		identity, identityErr := s.sesIdentity.Get(ctx, d.Name)
		if identityErr != nil {
			return fmt.Errorf("failed to read SES identity: %w", identityErr)
		}
		s.applySESIdentity(d, identity)
	} else {
		d.DKIMStatus = "pending"
	}

	txtRecords, err := net.LookupTXT(d.Name)
	if err == nil {
		spfCount := 0
		spfHasSES := false
		for _, record := range txtRecords {
			if isSPFRecord(record) {
				spfCount++
				spfHasSES = spfHasSES || containsSPF(record)
			}
		}
		if spfCount == 1 && spfHasSES {
			d.SPFStatus = "verified"
		} else if spfCount > 1 {
			d.SPFStatus = "failed"
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

	d.DMARCStatus = "failed"
	if d.DMARCDNSRecord != nil {
		if dmarcRecords, lookupErr := net.LookupTXT(*d.DMARCDNSRecord); lookupErr == nil {
			for _, record := range dmarcRecords {
				if containsDMARC(record) {
					d.DMARCStatus = "verified"
					break
				}
			}
		}
	}

	if outboundDomainVerified(d) {
		d.Status = domain.DomainStatusVerified
	}

	return s.domainRepo.UpdateForTeam(ctx, d)
}

func outboundDomainVerified(d *domain.Domain) bool {
	return d != nil && d.SPFStatus == "verified" && d.VerificationStatus == "verified" &&
		d.SESVerificationStatus == "verified" && d.DKIMStatus == "verified"
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
	if !isSPFRecord(record) {
		return false
	}
	for _, field := range strings.Fields(strings.ToLower(record)) {
		if field == "include:amazonses.com" {
			return true
		}
	}
	return false
}

func containsDMARC(record string) bool {
	fields := strings.Split(record, ";")
	versionFound := false
	policyFound := false
	for _, field := range fields {
		parts := strings.SplitN(strings.TrimSpace(field), "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(parts[0])) {
		case "v":
			versionFound = strings.EqualFold(strings.TrimSpace(parts[1]), "DMARC1")
		case "p":
			switch strings.ToLower(strings.TrimSpace(parts[1])) {
			case "none", "quarantine", "reject":
				policyFound = true
			}
		}
	}
	return versionFound && policyFound
}

func isSPFRecord(record string) bool {
	fields := strings.Fields(strings.TrimSpace(record))
	return len(fields) > 0 && strings.EqualFold(fields[0], "v=spf1")
}

func (s *DomainService) applySESIdentity(d *domain.Domain, identity *domain.SESIdentity) {
	if identity == nil {
		d.SESVerificationStatus = "pending"
		d.DKIMStatus = "pending"
		return
	}
	if identity.VerificationStatus != "" {
		d.SESVerificationStatus = strings.ToLower(identity.VerificationStatus)
	}
	if identity.VerifiedForSending {
		d.SESVerificationStatus = "verified"
	}
	if identity.DKIMStatus != "" {
		d.DKIMStatus = strings.ToLower(identity.DKIMStatus)
	}
	if d.DKIMStatus == "success" {
		d.DKIMStatus = "verified"
	}
	if len(identity.DKIMTokens) == 0 {
		return
	}
	hostedZone := strings.TrimSuffix(strings.TrimSpace(identity.SigningHostedZone), ".")
	if hostedZone == "" {
		hostedZone = "dkim.amazonses.com"
	}
	d.DKIMDNSRecords = make([]domain.DNSRecord, 0, len(identity.DKIMTokens))
	for _, token := range identity.DKIMTokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		d.DKIMDNSRecords = append(d.DKIMDNSRecords, domain.DNSRecord{
			Type:   domain.DNSRecordTypeCNAME,
			Host:   token + "._domainkey." + d.Name,
			Value:  token + "." + hostedZone,
			TTL:    3600,
			Status: d.DKIMStatus,
		})
	}
	if len(d.DKIMDNSRecords) > 0 {
		first := d.DKIMDNSRecords[0].Host
		d.DKIMDNSRecord = &first
	}
}
