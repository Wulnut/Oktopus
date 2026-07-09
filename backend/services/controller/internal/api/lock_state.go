package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/redis/go-redis/v9"
)

const defaultLockEvalTTL = 15 * time.Second

// unlockEvalScript deletes the eval lock only when the token still matches,
// so a TTL-expired unlock cannot remove another holder's key.
var unlockEvalScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0
`)

type lockDeviceState struct {
	LastIP      string              `json:"last_ip"`
	LastStatus  db.DeviceLockStatus `json:"last_status"`
	LastCommand string              `json:"last_command,omitempty"`
	NotifyOKAt  time.Time           `json:"notify_ok_at,omitempty"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type lockDeviceStateStore interface {
	Get(ctx context.Context, tenant, sn string) (lockDeviceState, bool, error)
	Put(ctx context.Context, tenant, sn string, st lockDeviceState) error
	TryLock(ctx context.Context, tenant, sn string, ttl time.Duration) (unlock func(), ok bool, err error)
}

var lockStateStore lockDeviceStateStore = noopLockDeviceStateStore{}

type noopLockDeviceStateStore struct{}

func (noopLockDeviceStateStore) Get(context.Context, string, string) (lockDeviceState, bool, error) {
	return lockDeviceState{}, false, nil
}

func (noopLockDeviceStateStore) Put(context.Context, string, string, lockDeviceState) error {
	return nil
}

// TryLock always succeeds with a no-op unlock so evaluate can proceed without mutual exclusion.
func (noopLockDeviceStateStore) TryLock(context.Context, string, string, time.Duration) (func(), bool, error) {
	return func() {}, true, nil
}

type redisLockDeviceStateStore struct {
	client *redis.Client
}

func lockDeviceStateKey(tenant, sn string) string {
	return fmt.Sprintf("oktopus:lock:state:%s:%s", tenant, db.NormalizeSN(sn))
}

func lockEvalKey(tenant, sn string) string {
	return fmt.Sprintf("oktopus:lock:eval:%s:%s", tenant, db.NormalizeSN(sn))
}

func (s *redisLockDeviceStateStore) Get(ctx context.Context, tenant, sn string) (lockDeviceState, bool, error) {
	raw, err := s.client.Get(ctx, lockDeviceStateKey(tenant, sn)).Bytes()
	if err == redis.Nil {
		return lockDeviceState{}, false, nil
	}
	if err != nil {
		return lockDeviceState{}, false, err
	}
	var st lockDeviceState
	if err := json.Unmarshal(raw, &st); err != nil {
		return lockDeviceState{}, false, err
	}
	return st, true, nil
}

func (s *redisLockDeviceStateStore) Put(ctx context.Context, tenant, sn string, st lockDeviceState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	// No TTL on state keys — last known IP/status should persist across restarts.
	return s.client.Set(ctx, lockDeviceStateKey(tenant, sn), raw, 0).Err()
}

// TryLock acquires a per-SN eval lock via SET NX EX with a unique token.
// Unlock is compare-and-del so TTL expiry cannot delete another holder's key.
// On Redis errors, soft-degrades: returns ok=true with a no-op unlock so the
// lock pipeline never hard-fails when Redis is unavailable. Contention
// (key already held) returns ok=false.
func (s *redisLockDeviceStateStore) TryLock(ctx context.Context, tenant, sn string, ttl time.Duration) (func(), bool, error) {
	if ttl <= 0 {
		ttl = defaultLockEvalTTL
	}
	key := lockEvalKey(tenant, sn)
	token := uuid.NewString()
	ok, err := s.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		log.Printf("lock_state: TryLock soft-degrade tenant=%s sn=%s: %v", tenant, sn, err)
		return func() {}, true, nil
	}
	if !ok {
		return nil, false, nil
	}
	unlock := func() {
		_, _ = unlockEvalScript.Run(context.Background(), s.client, []string{key}, token).Result()
	}
	return unlock, true, nil
}
