package api

import (
	"time"

	"github.com/leandrofars/oktopus/internal/db"
)

const (
	defaultLockBackoffThreshold = 3
	defaultLockBackoffCooldown  = 10 * time.Minute
	minLockBackoffCooldown      = 60 * time.Second
	maxLockBackoffErrorLen      = 512
)

// lockCommandBackoff tracks consecutive retryable delivery failures for one
// target state (LOCKED or UNLOCKED) of a single device.
type lockCommandBackoff struct {
	ConsecutiveFailures int       `json:"consecutive_failures"`
	CooldownUntil       time.Time `json:"cooldown_until,omitempty"`
	LastFailureAt       time.Time `json:"last_failure_at,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
}

// lockBackoffConfig is the resolved per-device failure-backoff policy.
type lockBackoffConfig struct {
	Enabled   bool
	Threshold int
	Cooldown  time.Duration
}

func defaultLockBackoffConfig() lockBackoffConfig {
	return lockBackoffConfig{
		Enabled:   false,
		Threshold: defaultLockBackoffThreshold,
		Cooldown:  defaultLockBackoffCooldown,
	}
}

// normalizeLockBackoffConfig applies the spec validation: threshold < 1 falls
// back to 3 and cooldown < 60s is clamped to 60s.
func normalizeLockBackoffConfig(cfg lockBackoffConfig) lockBackoffConfig {
	if cfg.Threshold < 1 {
		cfg.Threshold = defaultLockBackoffThreshold
	}
	if cfg.Cooldown < minLockBackoffCooldown {
		cfg.Cooldown = minLockBackoffCooldown
	}
	return cfg
}

// truncateBackoffError caps an error string to maxLockBackoffErrorLen runes for
// Redis/audit storage.
func truncateBackoffError(s string) string {
	r := []rune(s)
	if len(r) <= maxLockBackoffErrorLen {
		return s
	}
	return string(r[:maxLockBackoffErrorLen])
}

func backoffFor(m map[db.DeviceLockStatus]*lockCommandBackoff, status db.DeviceLockStatus) *lockCommandBackoff {
	if m == nil {
		return nil
	}
	return m[status]
}

// isBackoffCoolingDown reports whether status is within an active cooldown at now.
func isBackoffCoolingDown(m map[db.DeviceLockStatus]*lockCommandBackoff, status db.DeviceLockStatus, now time.Time) bool {
	b := backoffFor(m, status)
	return b != nil && now.Before(b.CooldownUntil)
}

// applyBackoffFailure returns a NEW map with one retryable failure recorded for
// status. It does not mutate prev. The failure count is never reset on cooldown
// expiry, so the first post-cooldown failure (count already >= threshold)
// immediately starts a new cooldown. Returns the new map and cooled=true when the
// threshold was reached (cooldown now active for status).
func applyBackoffFailure(
	prev map[db.DeviceLockStatus]*lockCommandBackoff,
	status db.DeviceLockStatus,
	now time.Time,
	threshold int,
	cooldown time.Duration,
	lastErr string,
) (map[db.DeviceLockStatus]*lockCommandBackoff, bool) {
	next := make(map[db.DeviceLockStatus]*lockCommandBackoff, len(prev))
	for k, v := range prev {
		next[k] = &lockCommandBackoff{
			ConsecutiveFailures: v.ConsecutiveFailures,
			CooldownUntil:       v.CooldownUntil,
			LastFailureAt:       v.LastFailureAt,
			LastError:           v.LastError,
		}
	}
	b := next[status]
	if b == nil {
		b = &lockCommandBackoff{}
		next[status] = b
	}
	b.ConsecutiveFailures++
	b.LastFailureAt = now
	b.LastError = truncateBackoffError(lastErr)
	cooled := false
	if threshold > 0 && b.ConsecutiveFailures >= threshold {
		b.CooldownUntil = now.Add(cooldown)
		cooled = true
	}
	return next, cooled
}

// maxBackoffFailures returns the highest ConsecutiveFailures across all targets.
// Used for the recovery audit's previous_max_failures field.
func maxBackoffFailures(m map[db.DeviceLockStatus]*lockCommandBackoff) int {
	max := 0
	for _, b := range m {
		if b.ConsecutiveFailures > max {
			max = b.ConsecutiveFailures
		}
	}
	return max
}
