package api

import (
	"testing"
)

func TestCircuitBreaker_DisabledNeverTrips(t *testing.T) {
	b := newLockCircuitBreaker()
	b.setEnabled(false)
	b.setThreshold(2, 60)

	// Record many locks — should never trip when disabled
	for i := 0; i < 100; i++ {
		b.recordLock("tenant-disabled")
	}
	if b.isTripped("tenant-disabled") {
		t.Fatal("disabled breaker should never trip")
	}
}

func TestCircuitBreaker_UnlockNeverRecorded(t *testing.T) {
	b := newLockCircuitBreaker()
	b.setThreshold(3, 60)

	// The breaker only tracks LOCK decisions via recordLock.
	// Verify that after threshold locks, a new tenant that only does
	// unlocks (never calls recordLock) is not affected.
	b.recordLock("tenant-locker")
	b.recordLock("tenant-locker")
	b.recordLock("tenant-locker")
	if !b.isTripped("tenant-locker") {
		t.Fatal("tenant-locker should be tripped after 3 locks")
	}

	// A tenant that never locked should not be tripped
	if b.isTripped("tenant-unlocker") {
		t.Fatal("tenant-unlocker should never be tripped if no locks recorded")
	}
}

func TestCircuitBreaker_ResetOnDifferentTenantDoesNotAffectOthers(t *testing.T) {
	b := newLockCircuitBreaker()
	b.setThreshold(2, 60)

	b.recordLock("tenant-a")
	b.recordLock("tenant-a")
	if !b.isTripped("tenant-a") {
		t.Fatal("tenant-a should be tripped")
	}

	// Resetting tenant-b should not affect tenant-a
	b.reset("tenant-b")
	if !b.isTripped("tenant-a") {
		t.Fatal("tenant-a should still be tripped after resetting tenant-b")
	}

	// Now reset tenant-a
	b.reset("tenant-a")
	if b.isTripped("tenant-a") {
		t.Fatal("tenant-a should be cleared after reset")
	}
}

func TestCircuitBreaker_ThresholdBoundary(t *testing.T) {
	b := newLockCircuitBreaker()
	b.setThreshold(5, 60)

	// Record exactly threshold-1: should not trip
	for i := 0; i < 4; i++ {
		b.recordLock("tenant-boundary")
	}
	if b.isTripped("tenant-boundary") {
		t.Fatal("should not trip below threshold")
	}

	// One more: exactly at threshold, should trip
	b.recordLock("tenant-boundary")
	if !b.isTripped("tenant-boundary") {
		t.Fatal("should trip exactly at threshold")
	}
}

func TestCircuitBreaker_EnabledByDefault(t *testing.T) {
	b := newLockCircuitBreaker()
	if !b.enabled {
		t.Fatal("circuit breaker should be enabled by default")
	}
}
