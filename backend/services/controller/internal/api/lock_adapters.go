package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/leandrofars/oktopus/internal/config"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const lockPolicyCacheTTL = 24 * time.Hour

type multiLockEventSink struct {
	sinks []lockEventSink
}

func (m multiLockEventSink) PublishLockAudit(ctx context.Context, tenantSlug string, entry db.LockAuditLog) error {
	for _, sink := range m.sinks {
		if err := sink.PublishLockAudit(ctx, tenantSlug, entry); err != nil {
			log.Printf("lock_adapters: publish audit sink: %v", err)
		}
	}
	return nil
}

type redisLockPolicyCache struct {
	client *redis.Client
}

// connectRedis parses url, creates a client, and pings to verify connectivity.
func connectRedis(url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func newRedisLockPolicyCache(url string) (*redisLockPolicyCache, error) {
	client, err := connectRedis(url)
	if err != nil {
		return nil, err
	}
	return &redisLockPolicyCache{client: client}, nil
}

func lockPolicyCacheKey(tenantSlug, sn string) string {
	return fmt.Sprintf("oktopus:lock:policy:%s:%s", tenantSlug, db.NormalizeSN(sn))
}

func (c *redisLockPolicyCache) GetLockPolicy(ctx context.Context, tenantSlug, sn string) (db.LockPolicy, bool, error) {
	raw, err := c.client.Get(ctx, lockPolicyCacheKey(tenantSlug, sn)).Bytes()
	if err == redis.Nil {
		return db.LockPolicy{}, false, nil
	}
	if err != nil {
		return db.LockPolicy{}, false, err
	}
	var policy db.LockPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return db.LockPolicy{}, false, err
	}
	return policy, true, nil
}

func (c *redisLockPolicyCache) PutLockPolicy(ctx context.Context, tenantSlug string, policy db.LockPolicy) error {
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, lockPolicyCacheKey(tenantSlug, policy.SN), raw, lockPolicyCacheTTL).Err()
}

func (c *redisLockPolicyCache) DeleteLockPolicy(ctx context.Context, tenantSlug, sn string) error {
	return c.client.Del(ctx, lockPolicyCacheKey(tenantSlug, sn)).Err()
}

type kafkaLockEventSink struct {
	writer *kafka.Writer
	topic  string
}

func newKafkaLockEventSink(brokers, topic string) (*kafkaLockEventSink, error) {
	parts := strings.Split(brokers, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("kafka brokers are required")
	}
	writer := &kafka.Writer{
		Addr:     kafka.TCP(parts...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	return &kafkaLockEventSink{writer: writer, topic: topic}, nil
}

type lockAuditKafkaMessage struct {
	TenantSlug  string             `json:"tenant_slug"`
	ID          string             `json:"id"`
	SN          string             `json:"sn,omitempty"`
	Action      string             `json:"action"`
	PolicyType  db.LockPolicyType  `json:"policy_type,omitempty"`
	Status      db.DeviceLockStatus `json:"status,omitempty"`
	OperatorID  string             `json:"operator_id,omitempty"`
	Description string             `json:"description,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
}

func lockAuditToKafkaMessage(tenantSlug string, entry db.LockAuditLog) lockAuditKafkaMessage {
	details := map[string]interface{}{}
	for k, v := range entry.Details {
		details[k] = v
	}
	return lockAuditKafkaMessage{
		TenantSlug:  tenantSlug,
		ID:          entry.ID.Hex(),
		SN:          entry.SN,
		Action:      entry.Action,
		PolicyType:  entry.PolicyType,
		Status:      entry.Status,
		OperatorID:  entry.OperatorID,
		Description: entry.Description,
		Details:     details,
		CreatedAt:   entry.CreatedAt,
	}
}

func (s *kafkaLockEventSink) PublishLockAudit(ctx context.Context, tenantSlug string, entry db.LockAuditLog) error {
	payload, err := json.Marshal(lockAuditToKafkaMessage(tenantSlug, entry))
	if err != nil {
		return err
	}
	return s.writer.WriteMessages(ctx, kafka.Message{Value: payload})
}

type greenplumLockEventSink struct {
	pool *pgxpool.Pool
}

func newGreenplumLockEventSink(dsn string) (*greenplumLockEventSink, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	sink := &greenplumLockEventSink{pool: pool}
	if err := sink.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return sink, nil
}

func (s *greenplumLockEventSink) ensureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS ont_lock_audit (
  id TEXT PRIMARY KEY,
  tenant_slug TEXT NOT NULL,
  sn TEXT,
  action TEXT NOT NULL,
  policy_type TEXT,
  status TEXT,
  operator_id TEXT,
  description TEXT,
  details JSONB,
  created_at TIMESTAMPTZ NOT NULL
)`)
	return err
}

func (s *greenplumLockEventSink) PublishLockAudit(ctx context.Context, tenantSlug string, entry db.LockAuditLog) error {
	id := entry.ID.Hex()
	if entry.ID.IsZero() {
		id = primitive.NewObjectID().Hex()
	}
	detailsJSON, err := json.Marshal(entry.Details)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO ont_lock_audit (id, tenant_slug, sn, action, policy_type, status, operator_id, description, details, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (id) DO NOTHING`,
		id,
		tenantSlug,
		entry.SN,
		entry.Action,
		string(entry.PolicyType),
		string(entry.Status),
		entry.OperatorID,
		entry.Description,
		detailsJSON,
		entry.CreatedAt,
	)
	return err
}

// InitLockScaleAdapters wires optional Redis/Kafka/Greenplum adapters from config.
// Core lock features continue to work when adapters are disabled.
func InitLockScaleAdapters(cfg config.LockScale) {
	if cfg.RedisEnabled {
		if cfg.RedisURL == "" {
			log.Printf("lock_adapters: LOCK_REDIS_ENABLED=true but LOCK_REDIS_URL is empty, using noop cache")
		} else if client, err := connectRedis(cfg.RedisURL); err != nil {
			log.Printf("lock_adapters: redis init failed: %v (using noop cache/state)", err)
		} else {
			lockCache = &redisLockPolicyCache{client: client}
			lockStateStore = &redisLockDeviceStateStore{client: client}
			log.Printf("lock_adapters: redis policy cache and device state store enabled")
		}
	}

	var sinks []lockEventSink
	if cfg.KafkaEnabled {
		if cfg.KafkaBrokers == "" {
			log.Printf("lock_adapters: LOCK_KAFKA_ENABLED=true but LOCK_KAFKA_BROKERS is empty, skipping kafka sink")
		} else if sink, err := newKafkaLockEventSink(cfg.KafkaBrokers, cfg.KafkaTopic); err != nil {
			log.Printf("lock_adapters: kafka sink init failed: %v", err)
		} else {
			sinks = append(sinks, sink)
			log.Printf("lock_adapters: kafka audit sink enabled (topic=%s)", cfg.KafkaTopic)
		}
	}
	if cfg.GreenplumEnabled {
		if cfg.GreenplumDSN == "" {
			log.Printf("lock_adapters: LOCK_GREENPLUM_ENABLED=true but LOCK_GREENPLUM_DSN is empty, skipping greenplum sink")
		} else if sink, err := newGreenplumLockEventSink(cfg.GreenplumDSN); err != nil {
			log.Printf("lock_adapters: greenplum sink init failed: %v", err)
		} else {
			sinks = append(sinks, sink)
			log.Printf("lock_adapters: greenplum audit sink enabled")
		}
	}
	if len(sinks) > 0 {
		lockAuditSink = multiLockEventSink{sinks: sinks}
	}
}

// setLockAdaptersForTest replaces global lock adapters in unit tests.
func setLockAdaptersForTest(cache lockPolicyCache, sink lockEventSink) {
	lockCache = cache
	lockAuditSink = sink
}

// setLockStateStoreForTest replaces the global device state store in unit tests.
func setLockStateStoreForTest(store lockDeviceStateStore) {
	lockStateStore = store
}

func resetLockAdaptersForTest() {
	lockCache = noopLockPolicyCache{}
	lockAuditSink = noopLockEventSink{}
	lockStateStore = noopLockDeviceStateStore{}
}
