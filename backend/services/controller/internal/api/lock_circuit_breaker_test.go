package api

import (
	"testing"
	"time"
)

func TestCircuitBreaker_NotTrippedBelowThreshold(t *testing.T) {
	b := newLockCircuitBreaker()
	b.setThreshold(5, 60)

	for i := 0; i < 4; i++ {
		b.recordLock("tenant-a")
	}
	if b.isTripped("tenant-a") {
		t.Fatal("breaker should not trip below threshold")
	}
}

func TestCircuitBreaker_TripsAtThreshold(t *testing.T) {
	b := newLockCircuitBreaker()
	b.setThreshold(3, 60)

	for i := 0; i < 3; i++ {
		b.recordLock("tenant-b")
	}
	if !b.isTripped("tenant-b") {
		t.Fatal("breaker should trip at threshold")
	}
}

func TestCircuitBreaker_ResetClearsTrippedState(t *testing.T) {
	b := newLockCircuitBreaker()
	b.setThreshold(2, 60)

	b.recordLock("tenant-c")
	b.recordLock("tenant-c")
	if !b.isTripped("tenant-c") {
		t.Fatal("breaker should be tripped")
	}

	b.reset("tenant-c")
	if b.isTripped("tenant-c") {
		t.Fatal("breaker should be cleared after reset")
	}
}

func TestCircuitBreaker_PerTenantIsolation(t *testing.T) {
	b := newLockCircuitBreaker()
	b.setThreshold(2, 60)

	b.recordLock("tenant-d")
	b.recordLock("tenant-d")
	if !b.isTripped("tenant-d") {
		t.Fatal("tenant-d should be tripped")
	}
	if b.isTripped("tenant-e") {
		t.Fatal("tenant-e should not be affected by tenant-d")
	}
}

func TestCircuitBreaker_WindowPrunesOldEntries(t *testing.T) {
	b := newLockCircuitBreaker()
	b.setThreshold(3, 1) // 1-second window

	// Record 2 locks
	b.recordLock("tenant-f")
	b.recordLock("tenant-f")

	// Wait for window to expire
	time.Sleep(1100 * time.Millisecond)

	// Record 1 more — should prune the old 2 and only have 1
	b.recordLock("tenant-f")
	if b.isTripped("tenant-f") {
		t.Fatal("breaker should not trip after window prune")
	}
}
