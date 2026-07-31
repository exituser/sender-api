package service

import "testing"

func TestGenerateSlugNormalizesSeparators(t *testing.T) {
	service := &TeamService{}
	if got := service.generateSlug("  Acme__Mail  "); got != "acme-mail" {
		t.Fatalf("expected normalized slug, got %q", got)
	}
}
