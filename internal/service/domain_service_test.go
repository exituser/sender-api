package service

import (
	"net"
	"testing"
)

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
