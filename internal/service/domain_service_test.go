package service

import (
	"context"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type sesIdentityProviderStub struct {
	identity *domain.SESIdentity
}

func (s *sesIdentityProviderStub) Create(context.Context, string) (*domain.SESIdentity, error) {
	return s.identity, nil
}

func (s *sesIdentityProviderStub) Get(context.Context, string) (*domain.SESIdentity, error) {
	return s.identity, nil
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
