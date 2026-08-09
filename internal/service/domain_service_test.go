package service

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type domainServiceRepoStub struct {
	domain *domain.Domain
}

func (r *domainServiceRepoStub) Create(context.Context, *domain.Domain) error { return nil }
func (r *domainServiceRepoStub) GetByID(context.Context, uuid.UUID) (*domain.Domain, error) {
	return r.domain, nil
}
func (r *domainServiceRepoStub) GetByIDForTeam(context.Context, uuid.UUID, uuid.UUID) (*domain.Domain, error) {
	return r.domain, nil
}
func (r *domainServiceRepoStub) GetByName(context.Context, uuid.UUID, string) (*domain.Domain, error) {
	return nil, nil
}
func (r *domainServiceRepoStub) List(context.Context, uuid.UUID) (*domain.DomainListResponse, error) {
	return &domain.DomainListResponse{Data: []domain.Domain{*r.domain}}, nil
}
func (r *domainServiceRepoStub) Update(context.Context, *domain.Domain) error        { return nil }
func (r *domainServiceRepoStub) UpdateForTeam(context.Context, *domain.Domain) error { return nil }
func (r *domainServiceRepoStub) Delete(context.Context, uuid.UUID) error             { return nil }
func (r *domainServiceRepoStub) DeleteForTeam(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (r *domainServiceRepoStub) GetTeamByDomain(context.Context, string) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func TestDomainServiceReconstructsDNSSetupForListAndGet(t *testing.T) {
	verificationHost := "_sender-api-verification.example.com"
	dmarcHost := "_dmarc.example.com"
	spf := "v=spf1 include:amazonses.com ~all"
	mx := "inbound-smtp.eu-west-1.amazonaws.com"
	token := "sender-api-verify-token"
	repo := &domainServiceRepoStub{domain: &domain.Domain{
		ID: uuid.New(), Name: "example.com", VerificationToken: token,
		VerificationDNSRecord: &verificationHost, SPFDNSRecord: &spf, MXDNSRecord: &mx,
		DMARCDNSRecord: &dmarcHost, VerificationStatus: "pending", SPFStatus: "pending",
		MXStatus: "pending", DMARCStatus: "pending", CreatedAt: time.Now(),
		DKIMDNSRecords: []domain.DNSRecord{{Type: domain.DNSRecordTypeCNAME, Host: "a._domainkey.example.com", Value: "a.dkim.amazonses.com", TTL: 3600, Status: "pending"}},
	}}
	service := NewDomainService(repo, nil, "eu-west-1")

	listed, err := service.List(context.Background(), uuid.New())
	if err != nil || len(listed.Data) != 1 {
		t.Fatalf("List() error=%v response=%+v", err, listed)
	}
	got := listed.Data[0].DNSRecords
	if len(got) != 5 || got[2].Host != "_sender-api-verification" || got[2].Value != token || got[3].Value != "v=DMARC1; p=none" {
		t.Fatalf("unexpected reconstructed list records: %+v", got)
	}

	fetched, err := service.GetByID(context.Background(), uuid.New(), repo.domain.ID)
	if err != nil || len(fetched.DNSRecords) != len(got) {
		t.Fatalf("GetByID() error=%v domain=%+v", err, fetched)
	}
}

func TestBuildDNSRecordsReconstructsPersistedSetup(t *testing.T) {
	spf := "v=spf1 include:amazonses.com ~all"
	mx := "inbound-smtp.eu-west-1.amazonaws.com"
	verificationHost := "_sender-api-verification.example.com"
	dmarcHost := "_dmarc.example.com"
	d := &domain.Domain{
		Name:                  "example.com",
		VerificationToken:     "sender-api-verify-token",
		VerificationStatus:    "pending",
		SPFStatus:             "pending",
		MXStatus:              "pending",
		DMARCStatus:           "pending",
		SPFDNSRecord:          &spf,
		MXDNSRecord:           &mx,
		VerificationDNSRecord: &verificationHost,
		DMARCDNSRecord:        &dmarcHost,
		DKIMDNSRecords: []domain.DNSRecord{{
			Type: domain.DNSRecordTypeCNAME, Host: "token._domainkey.example.com", Value: "token.dkim.amazonses.com", TTL: 3600,
		}},
	}

	records := buildDNSRecords(d)
	if len(records) != 5 {
		t.Fatalf("expected five setup records, got %d: %+v", len(records), records)
	}
	if records[2].Host != "_sender-api-verification" || records[2].Value != d.VerificationToken {
		t.Fatalf("verification record was not reconstructed: %+v", records[2])
	}
	if records[3].Host != "_dmarc" || !records[1].Optional {
		t.Fatalf("persisted optional or relative host fields were not reconstructed: %+v", records)
	}
}

func TestApplySESIdentityBuildsAllDKIMRecords(t *testing.T) {
	service := NewDomainService(nil, nil, "eu-west-1")
	d := &domain.Domain{ID: uuid.New(), Name: "example.com", DKIMStatus: "pending"}
	service.applySESIdentity(d, &domain.SESIdentity{
		VerifiedForSending: true,
		VerificationStatus: "success",
		DKIMStatus:         "success",
		DKIMTokens:         []string{"token-a", "token-b", "token-c"},
		SigningHostedZone:  "dkim.amazonses.com.",
	})
	if d.SESVerificationStatus != "verified" || d.DKIMStatus != "verified" {
		t.Fatalf("unexpected SES statuses: ses=%q dkim=%q", d.SESVerificationStatus, d.DKIMStatus)
	}
	if len(d.DKIMDNSRecords) != 3 || d.DKIMDNSRecords[1].Host != "token-b._domainkey.example.com" {
		t.Fatalf("unexpected DKIM records: %+v", d.DKIMDNSRecords)
	}
}

func TestContainsInboundMX(t *testing.T) {
	tests := []struct {
		name    string
		region  string
		records []*net.MX
		want    bool
	}{
		{
			name:    "accepts SES endpoint with trailing dot",
			region:  "eu-west-1",
			records: []*net.MX{{Host: "inbound-smtp.eu-west-1.amazonaws.com.", Pref: 10}},
			want:    true,
		},
		{
			name:    "accepts China SES endpoint",
			region:  "cn-north-1",
			records: []*net.MX{{Host: "inbound-smtp.cn-north-1.amazonaws.com.cn.", Pref: 10}},
			want:    true,
		},
		{
			name:    "rejects another provider",
			region:  "eu-west-1",
			records: []*net.MX{{Host: "route1.mx.cloudflare.net.", Pref: 10}},
			want:    false,
		},
		{
			name:    "rejects empty region",
			region:  "",
			records: []*net.MX{{Host: "inbound-smtp.eu-west-1.amazonaws.com.", Pref: 10}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsInboundMX(tt.records, tt.region); got != tt.want {
				t.Fatalf("containsInboundMX() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutboundDomainVerificationDoesNotRequireMX(t *testing.T) {
	d := &domain.Domain{
		SPFStatus:             "verified",
		VerificationStatus:    "verified",
		SESVerificationStatus: "verified",
		DKIMStatus:            "verified",
		MXStatus:              "failed",
	}
	if !outboundDomainVerified(d) {
		t.Fatal("outbound domain verification must not require SES inbound MX")
	}
}

func TestSPFAndDMARCParsingRejectAmbiguousOrIncompleteRecords(t *testing.T) {
	if !containsSPF("v=spf1 include:amazonses.com ~all") {
		t.Fatal("expected SES SPF mechanism to be recognized")
	}
	if containsSPF("v=spf1 include:other.example ~all") {
		t.Fatal("unexpected SPF verification without SES include")
	}
	if isSPFRecord("v=spf2 include:amazonses.com") {
		t.Fatal("unexpected non-SPF1 record")
	}
	if !containsDMARC("v=DMARC1; p=quarantine; rua=mailto:dmarc@example.com") {
		t.Fatal("expected DMARC policy to be recognized")
	}
	if containsDMARC("v=DMARC1; rua=mailto:dmarc@example.com") {
		t.Fatal("expected DMARC without policy to fail")
	}
}
