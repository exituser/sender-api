package queue

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUsageKeyUsesReservationDate(t *testing.T) {
	teamID := uuid.New()
	reservedAt := time.Date(2026, time.August, 7, 23, 59, 59, 0, time.UTC)
	releasedAt := reservedAt.Add(2 * time.Second)

	reservedKey, _ := usageKey(teamID, reservedAt)
	releaseKey, _ := usageKey(teamID, releasedAt)
	if reservedKey == releaseKey {
		t.Fatalf("expected different daily keys across UTC midnight, got %q", reservedKey)
	}
	if want := ":2026-08-07"; !strings.HasSuffix(reservedKey, want) {
		t.Fatalf("reservation key has wrong date: %q", reservedKey)
	}
}
