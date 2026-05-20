package api

import (
	"testing"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
)

func TestCampaignWindowOccurrenceKey_SameDayInside(t *testing.T) {
	c := db.Campaign{
		TimeWindowStart: "07:20",
		TimeWindowEnd:   "07:30",
	}
	now := time.Date(2026, 5, 20, 7, 25, 0, 0, time.UTC)
	key := campaignWindowOccurrenceKey(c, now)
	want := "2026-05-20:07:20:07:30"
	if key != want {
		t.Fatalf("got %q want %q", key, want)
	}
}

func TestCampaignWindowOccurrenceKey_SameDayOutside(t *testing.T) {
	c := db.Campaign{
		TimeWindowStart: "07:20",
		TimeWindowEnd:   "07:30",
	}
	now := time.Date(2026, 5, 20, 7, 10, 0, 0, time.UTC)
	if key := campaignWindowOccurrenceKey(c, now); key != "" {
		t.Fatalf("expected empty key outside window, got %q", key)
	}
}

func TestCampaignWindowOccurrenceKey_EndExclusive(t *testing.T) {
	c := db.Campaign{
		TimeWindowStart: "07:20",
		TimeWindowEnd:   "07:30",
	}
	now := time.Date(2026, 5, 20, 7, 30, 0, 0, time.UTC)
	if key := campaignWindowOccurrenceKey(c, now); key != "" {
		t.Fatalf("expected empty key at end boundary, got %q", key)
	}
}

func TestCampaignWindowOccurrenceKey_OvernightAfterMidnight(t *testing.T) {
	c := db.Campaign{
		TimeWindowStart: "22:00",
		TimeWindowEnd:   "06:00",
	}
	now := time.Date(2026, 5, 21, 2, 0, 0, 0, time.UTC)
	key := campaignWindowOccurrenceKey(c, now)
	want := "2026-05-20:22:00:06:00"
	if key != want {
		t.Fatalf("got %q want %q", key, want)
	}
}

func TestCampaignWindowOccurrenceKey_OvernightBeforeMidnight(t *testing.T) {
	c := db.Campaign{
		TimeWindowStart: "22:00",
		TimeWindowEnd:   "06:00",
	}
	now := time.Date(2026, 5, 20, 23, 0, 0, 0, time.UTC)
	key := campaignWindowOccurrenceKey(c, now)
	want := "2026-05-20:22:00:06:00"
	if key != want {
		t.Fatalf("got %q want %q", key, want)
	}
}

func TestCampaignHasTimeWindow(t *testing.T) {
	if campaignHasTimeWindow(db.Campaign{}) {
		t.Fatal("expected no window when empty")
	}
	if !campaignHasTimeWindow(db.Campaign{TimeWindowStart: "00:00", TimeWindowEnd: "06:00"}) {
		t.Fatal("expected window when both set")
	}
}

func TestIsWithinTimeWindowAt(t *testing.T) {
	c := db.Campaign{TimeWindowStart: "07:20", TimeWindowEnd: "07:30"}
	inside := time.Date(2026, 5, 20, 7, 25, 0, 0, time.UTC)
	if !isWithinTimeWindowAt(c, inside) {
		t.Fatal("expected inside window")
	}
	outside := time.Date(2026, 5, 20, 7, 10, 0, 0, time.UTC)
	if isWithinTimeWindowAt(c, outside) {
		t.Fatal("expected outside window")
	}
}
