package api

import (
	"testing"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
)

func TestShouldSkipIPPollForNotifyHealth(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	health := 120 * time.Second

	tests := []struct {
		name       string
		notifyOKAt time.Time
		health     time.Duration
		want       bool
	}{
		{name: "zero notify", notifyOKAt: time.Time{}, health: health, want: false},
		{name: "zero health", notifyOKAt: now.Add(-30 * time.Second), health: 0, want: false},
		{name: "within window", notifyOKAt: now.Add(-30 * time.Second), health: health, want: true},
		{name: "at boundary exclusive", notifyOKAt: now.Add(-health), health: health, want: false},
		{name: "outside window", notifyOKAt: now.Add(-121 * time.Second), health: health, want: false},
		{name: "future notify soft-ignore", notifyOKAt: now.Add(10 * time.Second), health: health, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipIPPollForNotifyHealth(tt.notifyOKAt, now, tt.health)
			if got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestShouldSkipIPPollForSameIP(t *testing.T) {
	tests := []struct {
		name      string
		lastIP    string
		currentIP string
		want      bool
	}{
		{name: "same", lastIP: "1.2.3.4", currentIP: "1.2.3.4", want: true},
		{name: "different", lastIP: "1.2.3.4", currentIP: "5.6.7.8", want: false},
		{name: "empty last", lastIP: "", currentIP: "1.2.3.4", want: false},
		{name: "empty current", lastIP: "1.2.3.4", currentIP: "", want: false},
		{name: "both empty", lastIP: "", currentIP: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipIPPollForSameIP(tt.lastIP, tt.currentIP)
			if got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestShouldSkipIPPollForUnsupported(t *testing.T) {
	tests := []struct {
		name   string
		hasRow bool
		optOut bool
		want   bool
	}{
		{name: "no row", hasRow: false, optOut: false, want: false},
		{name: "unsupported", hasRow: true, optOut: false, want: true},
		{name: "opt out", hasRow: true, optOut: true, want: true},
		{name: "opt out without row ignored", hasRow: false, optOut: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipIPPollForUnsupported(tt.hasRow, tt.optOut)
			if got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestHasPendingLockBackoff_BypassesPollShortcutsOnlyWhenEnabled(t *testing.T) {
	state := lockDeviceState{
		CommandBackoffs: map[db.DeviceLockStatus]*lockCommandBackoff{
			db.LockStatusLocked: {ConsecutiveFailures: 3, CooldownUntil: time.Now().Add(time.Minute)},
		},
	}
	if !hasPendingLockBackoff(true, state) {
		t.Fatal("enabled backoff state must bypass notify-health and same-IP poll shortcuts")
	}
	if hasPendingLockBackoff(false, state) {
		t.Fatal("disabled backoff must preserve legacy poll shortcuts")
	}
	if hasPendingLockBackoff(true, lockDeviceState{}) {
		t.Fatal("empty backoff state must preserve poll shortcuts")
	}
}
