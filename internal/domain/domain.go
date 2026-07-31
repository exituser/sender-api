package domain

import (
	"time"

	"github.com/google/uuid"
)

type DomainStatus string

const (
	DomainStatusPending  DomainStatus = "pending"
	DomainStatusVerified DomainStatus = "verified"
	DomainStatusFailed   DomainStatus = "failed"
)

type DNSRecordType string

const (
	DNSRecordTypeTXT   DNSRecordType = "TXT"
	DNSRecordTypeCNAME DNSRecordType = "CNAME"
	DNSRecordTypeMX    DNSRecordType = "MX"
)

type Domain struct {
	ID                    uuid.UUID    `json:"id" db:"id"`
	TeamID                uuid.UUID    `json:"team_id" db:"team_id"`
	Name                  string       `json:"name" db:"name"`
	Status                DomainStatus `json:"status" db:"status"`
	VerificationToken     string       `json:"-" db:"verification_token"`
	VerificationStatus    string       `json:"verification_status" db:"verification_status"`
	SPFStatus             string       `json:"spf_status" db:"spf_status"`
	MXStatus              string       `json:"mx_status" db:"mx_status"`
	DKIMStatus            string       `json:"dkim_status" db:"dkim_status"`
	DMARCStatus           string       `json:"dmarc_status" db:"dmarc_status"`
	DKIMDNSRecord         *string      `json:"dkim_dns_record,omitempty" db:"dkim_dns_record"`
	SPFDNSRecord          *string      `json:"spf_dns_record,omitempty" db:"spf_dns_record"`
	MXDNSRecord           *string      `json:"mx_dns_record,omitempty" db:"mx_dns_record"`
	DMARCDNSRecord        *string      `json:"dmarc_dns_record,omitempty" db:"dmarc_dns_record"`
	VerificationDNSRecord *string      `json:"verification_dns_record,omitempty" db:"verification_dns_record"`
	CreatedAt             time.Time    `json:"created_at" db:"created_at"`
}

type DNSRecord struct {
	Type   DNSRecordType `json:"type"`
	Host   string        `json:"host"`
	Value  string        `json:"value"`
	TTL    int           `json:"ttl"`
	Status string        `json:"status"`
}

type CreateDomainRequest struct {
	Name string `json:"name"`
}

type DomainResponse struct {
	ID           uuid.UUID    `json:"id"`
	Name         string       `json:"name"`
	Status       DomainStatus `json:"status"`
	DNSRecords   []DNSRecord  `json:"dns_records"`
	Instructions string       `json:"instructions"`
	CreatedAt    time.Time    `json:"created_at"`
}

type DomainListResponse struct {
	Data []Domain `json:"data"`
}
