package config

import (
	"os"
	"testing"
	"time"
)

func clearBackoffEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"LOCK_DEVICE_FAILURE_BACKOFF_ENABLED",
		"LOCK_DEVICE_FAILURE_THRESHOLD",
		"LOCK_DEVICE_FAILURE_COOLDOWN_SEC",
	} {
		os.Unsetenv(k)
	}
}

func TestParseLockDeviceFailureBackoff_DefaultsDisabled(t *testing.T) {
	clearBackoffEnv(t)
	cfg := parseLockDeviceFailureBackoff()
	if cfg.Enabled {
		t.Fatal("default must be disabled")
	}
	if cfg.Threshold != 3 {
		t.Fatalf("default threshold = %d, want 3", cfg.Threshold)
	}
	if cfg.Cooldown != 600*time.Second {
		t.Fatalf("default cooldown = %v, want 600s", cfg.Cooldown)
	}
}

func TestParseLockDeviceFailureBackoff_Enabled(t *testing.T) {
	clearBackoffEnv(t)
	t.Setenv("LOCK_DEVICE_FAILURE_BACKOFF_ENABLED", "true")
	t.Setenv("LOCK_DEVICE_FAILURE_THRESHOLD", "5")
	t.Setenv("LOCK_DEVICE_FAILURE_COOLDOWN_SEC", "120")
	cfg := parseLockDeviceFailureBackoff()
	if !cfg.Enabled || cfg.Threshold != 5 || cfg.Cooldown != 120*time.Second {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}
