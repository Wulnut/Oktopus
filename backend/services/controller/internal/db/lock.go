package db

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type LockPolicyType string

const (
	LockPolicyWhitelist LockPolicyType = "WHITELIST"
)

type DeviceLockStatus string

const (
	LockStatusLocked   DeviceLockStatus = "LOCKED"
	LockStatusUnlocked DeviceLockStatus = "UNLOCKED"
	LockStatusPending  DeviceLockStatus = "PENDING"
)

type LockReason string

const (
	LockReasonMasterDisabled LockReason = "MASTER_DISABLED"
	LockReasonAuthorized     LockReason = "AUTHORIZED"
	LockReasonUnauthorized   LockReason = "UNAUTHORIZED"
	LockReasonInvalidIP      LockReason = "INVALID_IP"
)

type LockCommandStatus string

const (
	LockCommandPending LockCommandStatus = "pending"
	LockCommandSuccess LockCommandStatus = "success"
	LockCommandFailed  LockCommandStatus = "failed"
	LockCommandRetry   LockCommandStatus = "retry"
)

const DefaultLockCommandMaxAttempts = 10

type LockPolicy struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"             json:"id"`
	SN             string             `bson:"sn"                        json:"sn"`
	PolicyType     LockPolicyType     `bson:"policy_type"               json:"policy_type"`
	AllowedIPRange string             `bson:"allowed_ip_range,omitempty" json:"allowed_ip_range,omitempty"`
	Description    string             `bson:"description,omitempty"      json:"description,omitempty"`
	OperatorID     string             `bson:"operator_id,omitempty"      json:"operator_id,omitempty"`
	Status         bool               `bson:"status"                    json:"status"`
	CreatedAt      time.Time          `bson:"created_at"                json:"created_at"`
	UpdatedAt      time.Time          `bson:"updated_at"                json:"updated_at"`
}

type LockConfig struct {
	ID              string    `bson:"_id"                json:"id"`
	MasterEnabled   bool      `bson:"master_enabled"     json:"master_enabled"`
	AutoLockEnabled bool      `bson:"auto_lock_enabled"  json:"auto_lock_enabled"`
	UpdatedBy       string    `bson:"updated_by,omitempty" json:"updated_by,omitempty"`
	UpdatedAt       time.Time `bson:"updated_at"         json:"updated_at"`
}

type UnauthorizedDevice struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"        json:"id"`
	SN         string             `bson:"sn"                   json:"sn"`
	ReportedIP string             `bson:"reported_ip"          json:"reported_ip"`
	Reason     LockReason         `bson:"reason"               json:"reason"`
	Status     DeviceLockStatus   `bson:"status"               json:"status"`
	FirstSeen  time.Time          `bson:"first_seen"           json:"first_seen"`
	LastSeen   time.Time          `bson:"last_seen"            json:"last_seen"`
}

// UnsupportedLockDevice tracks devices whose OntLock vendor paths are missing
// from the device schema (USP 7026 / path-not-in-schema). Operators can opt out
// of further probing via OptOut.
type UnsupportedLockDevice struct {
	SN            string    `bson:"sn" json:"sn"`
	Reason        string    `bson:"reason" json:"reason"` // unsupported_path
	Detail        string    `bson:"detail,omitempty" json:"detail,omitempty"`
	OptOut        bool      `bson:"opt_out" json:"opt_out"`
	LastCheckedAt time.Time `bson:"last_checked_at" json:"last_checked_at"`
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at" json:"updated_at"`
	OperatorID    string    `bson:"operator_id,omitempty" json:"operator_id,omitempty"`
}

const LockUnsupportedReasonPath = "unsupported_path"

type LockCommandAttempt struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"        json:"id"`
	CommandID    string             `bson:"command_id"           json:"command_id"`
	DeviceSN     string             `bson:"device_sn"            json:"device_sn"`
	ReportedIP   string             `bson:"reported_ip,omitempty" json:"reported_ip,omitempty"`
	TargetStatus DeviceLockStatus   `bson:"target_status"        json:"target_status"`
	CommandValue string             `bson:"command_value"        json:"command_value"`
	Status       LockCommandStatus  `bson:"status"               json:"status"`
	Error        string             `bson:"error,omitempty"      json:"error,omitempty"`
	AttemptCount int                `bson:"attempt_count"        json:"attempt_count"`
	CreatedAt    time.Time          `bson:"created_at"           json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at"           json:"updated_at"`
}

type LockAuditLog struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"       json:"id"`
	SN          string             `bson:"sn,omitempty"        json:"sn,omitempty"`
	Action      string             `bson:"action"              json:"action"`
	PolicyType  LockPolicyType     `bson:"policy_type,omitempty" json:"policy_type,omitempty"`
	Status      DeviceLockStatus   `bson:"status,omitempty"    json:"status,omitempty"`
	OperatorID  string             `bson:"operator_id,omitempty" json:"operator_id,omitempty"`
	Description string             `bson:"description,omitempty" json:"description,omitempty"`
	Details     bson.M             `bson:"details,omitempty"   json:"details,omitempty"`
	CreatedAt   time.Time          `bson:"created_at"          json:"created_at"`
}

func (t *TenantDB) LockPolicies() *mongo.Collection {
	return t.General.Collection("device_lock_policy")
}
func (t *TenantDB) LockConfig() *mongo.Collection { return t.General.Collection("device_lock_config") }
func (t *TenantDB) UnauthorizedDevices() *mongo.Collection {
	return t.General.Collection("lock_unauthorized_devices")
}
func (t *TenantDB) UnsupportedLockDevices() *mongo.Collection {
	return t.General.Collection("lock_unsupported_devices")
}
func (t *TenantDB) LockCommands() *mongo.Collection {
	return t.General.Collection("lock_command_attempts")
}
func (t *TenantDB) LockAuditLogs() *mongo.Collection { return t.General.Collection("lock_audit_logs") }

func DefaultLockConfig() LockConfig {
	return LockConfig{
		ID:              "global",
		MasterEnabled:   true,
		AutoLockEnabled: true,
		UpdatedAt:       time.Now(),
	}
}

func NormalizeSN(sn string) string {
	return strings.ToUpper(strings.TrimSpace(sn))
}

func (p *LockPolicy) Normalize() {
	p.SN = NormalizeSN(p.SN)
	p.AllowedIPRange = strings.TrimSpace(p.AllowedIPRange)
	p.Description = strings.TrimSpace(p.Description)
	p.OperatorID = strings.TrimSpace(p.OperatorID)
}

func (p LockPolicy) Validate() error {
	if p.SN == "" {
		return errors.New("sn is required")
	}
	if p.AllowedIPRange == "" {
		return errors.New("allowed_ip_range is required")
	}
	if _, _, err := net.ParseCIDR(p.AllowedIPRange); err != nil {
		return err
	}
	return nil
}

func (t *TenantDB) UpsertLockPolicy(ctx context.Context, p LockPolicy) (LockPolicy, error) {
	p.Normalize()
	if err := p.Validate(); err != nil {
		return p, err
	}
	now := time.Now()
	if p.ID.IsZero() {
		p.ID = primitive.NewObjectID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	update := bson.M{
		"$set": bson.M{
			"policy_type":      p.PolicyType,
			"allowed_ip_range": p.AllowedIPRange,
			"description":      p.Description,
			"operator_id":      p.OperatorID,
			"status":           p.Status,
			"updated_at":       p.UpdatedAt,
		},
		// Drop legacy blacklist-only field if present on older documents.
		"$unset": bson.M{
			"reason_code": "",
		},
		"$setOnInsert": bson.M{
			"_id":        p.ID,
			"sn":         p.SN,
			"created_at": p.CreatedAt,
		},
	}
	var out LockPolicy
	err := t.LockPolicies().FindOneAndUpdate(ctx, bson.M{"sn": p.SN}, update, opts).Decode(&out)
	return out, err
}

func (t *TenantDB) GetLockPolicy(ctx context.Context, sn string) (LockPolicy, error) {
	var p LockPolicy
	err := t.LockPolicies().FindOne(ctx, bson.M{"sn": NormalizeSN(sn), "status": true}).Decode(&p)
	return p, err
}

func (t *TenantDB) ListLockPolicies(ctx context.Context, policyType LockPolicyType) ([]LockPolicy, error) {
	filter := bson.M{}
	if policyType != "" {
		filter["policy_type"] = policyType
	}
	cursor, err := t.LockPolicies().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var policies []LockPolicy
	err = cursor.All(ctx, &policies)
	if policies == nil {
		policies = []LockPolicy{}
	}
	return policies, err
}

func (t *TenantDB) DeleteLockPolicy(ctx context.Context, sn string) error {
	_, err := t.LockPolicies().DeleteOne(ctx, bson.M{"sn": NormalizeSN(sn)})
	return err
}

func (t *TenantDB) DeleteLockPolicies(ctx context.Context, sns []string) (int64, error) {
	if len(sns) == 0 {
		return 0, nil
	}
	normalized := make([]string, 0, len(sns))
	for _, sn := range sns {
		normalized = append(normalized, NormalizeSN(sn))
	}
	res, err := t.LockPolicies().DeleteMany(ctx, bson.M{"sn": bson.M{"$in": normalized}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

func (t *TenantDB) GetLockConfig(ctx context.Context) (LockConfig, error) {
	var cfg LockConfig
	err := t.LockConfig().FindOne(ctx, bson.M{"_id": "global"}).Decode(&cfg)
	if err == mongo.ErrNoDocuments {
		return DefaultLockConfig(), nil
	}
	return cfg, err
}

func (t *TenantDB) SaveLockConfig(ctx context.Context, cfg LockConfig) (LockConfig, error) {
	if cfg.ID == "" {
		cfg.ID = "global"
	}
	cfg.UpdatedAt = time.Now()
	_, err := t.LockConfig().UpdateOne(ctx,
		bson.M{"_id": cfg.ID},
		bson.M{"$set": bson.M{
			"master_enabled":    cfg.MasterEnabled,
			"auto_lock_enabled": cfg.AutoLockEnabled,
			"updated_by":        cfg.UpdatedBy,
			"updated_at":        cfg.UpdatedAt,
		}},
		options.Update().SetUpsert(true),
	)
	return cfg, err
}

func (t *TenantDB) RecordUnauthorizedDevice(ctx context.Context, d UnauthorizedDevice) error {
	d.SN = NormalizeSN(d.SN)
	now := time.Now()
	if d.FirstSeen.IsZero() {
		d.FirstSeen = now
	}
	d.LastSeen = now
	if d.Status == "" {
		d.Status = LockStatusPending
	}
	_, err := t.UnauthorizedDevices().UpdateOne(ctx,
		bson.M{"sn": d.SN},
		bson.M{
			"$set": bson.M{
				"reported_ip": d.ReportedIP,
				"reason":      d.Reason,
				"status":      d.Status,
				"last_seen":   d.LastSeen,
			},
			"$setOnInsert": bson.M{
				"_id":        primitive.NewObjectID(),
				"sn":         d.SN,
				"first_seen": d.FirstSeen,
			},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (t *TenantDB) ListUnauthorizedDevices(ctx context.Context) ([]UnauthorizedDevice, error) {
	cursor, err := t.UnauthorizedDevices().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "last_seen", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var devices []UnauthorizedDevice
	err = cursor.All(ctx, &devices)
	if devices == nil {
		devices = []UnauthorizedDevice{}
	}
	return devices, err
}

func (t *TenantDB) CreateLockCommandAttempt(ctx context.Context, c LockCommandAttempt) (LockCommandAttempt, error) {
	now := time.Now()
	c.ID = primitive.NewObjectID()
	if c.CommandID == "" {
		c.CommandID = uuid.NewString()
	}
	if c.Status == "" {
		c.Status = LockCommandPending
	}
	c.AttemptCount++
	c.CreatedAt = now
	c.UpdatedAt = now
	_, err := t.LockCommands().InsertOne(ctx, c)
	return c, err
}

func (t *TenantDB) UpdateLockCommandStatus(ctx context.Context, id primitive.ObjectID, status LockCommandStatus, errMsg string) error {
	_, err := t.LockCommands().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"status":     status,
			"error":      errMsg,
			"updated_at": time.Now(),
		}},
	)
	return err
}

func (t *TenantDB) MarkLockCommandForRetry(ctx context.Context, id primitive.ObjectID, errMsg string) error {
	_, err := t.LockCommands().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"status":     LockCommandRetry,
			"error":      errMsg,
			"updated_at": time.Now(),
		}},
	)
	return err
}

func (t *TenantDB) PrepareLockCommandResend(ctx context.Context, id primitive.ObjectID) (LockCommandAttempt, error) {
	var attempt LockCommandAttempt
	err := t.LockCommands().FindOneAndUpdate(ctx,
		bson.M{"_id": id},
		bson.M{
			"$inc": bson.M{"attempt_count": 1},
			"$set": bson.M{
				"status":     LockCommandPending,
				"error":      "",
				"updated_at": time.Now(),
			},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&attempt)
	return attempt, err
}

func (t *TenantDB) ListRetryableLockCommands(ctx context.Context, retryAfter time.Time, limit int64) ([]LockCommandAttempt, error) {
	if limit <= 0 {
		limit = 100
	}
	filter := bson.M{
		"status":     LockCommandRetry,
		"updated_at": bson.M{"$lte": retryAfter},
	}
	cursor, err := t.LockCommands().Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: 1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	var commands []LockCommandAttempt
	err = cursor.All(ctx, &commands)
	return commands, err
}

func (t *TenantDB) DeleteUnauthorizedDevice(ctx context.Context, sn string) error {
	_, err := t.UnauthorizedDevices().DeleteOne(ctx, bson.M{"sn": NormalizeSN(sn)})
	return err
}

// UpsertUnsupportedLockDevice records or refreshes an OntLock-unsupported device.
// OptOut / OperatorID are preserved on update unless explicitly set on insert.
func (t *TenantDB) UpsertUnsupportedLockDevice(ctx context.Context, d UnsupportedLockDevice) error {
	d.SN = NormalizeSN(d.SN)
	if d.SN == "" {
		return errors.New("sn is required")
	}
	if d.Reason == "" {
		d.Reason = LockUnsupportedReasonPath
	}
	now := time.Now()
	if d.LastCheckedAt.IsZero() {
		d.LastCheckedAt = now
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	_, err := t.UnsupportedLockDevices().UpdateOne(ctx,
		bson.M{"sn": d.SN},
		bson.M{
			"$set": bson.M{
				"reason":          d.Reason,
				"detail":          d.Detail,
				"last_checked_at": d.LastCheckedAt,
				"updated_at":      d.UpdatedAt,
			},
			"$setOnInsert": bson.M{
				"sn":          d.SN,
				"opt_out":     false,
				"created_at":  d.CreatedAt,
				"operator_id": "",
			},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (t *TenantDB) GetUnsupportedLockDevice(ctx context.Context, sn string) (UnsupportedLockDevice, error) {
	var d UnsupportedLockDevice
	err := t.UnsupportedLockDevices().FindOne(ctx, bson.M{"sn": NormalizeSN(sn)}).Decode(&d)
	return d, err
}

func (t *TenantDB) ListUnsupportedLockDevices(ctx context.Context) ([]UnsupportedLockDevice, error) {
	cursor, err := t.UnsupportedLockDevices().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var devices []UnsupportedLockDevice
	err = cursor.All(ctx, &devices)
	return devices, err
}

// SetUnsupportedLockOptOut sets or clears the opt-out flag. The row must already exist.
func (t *TenantDB) SetUnsupportedLockOptOut(ctx context.Context, sn string, optOut bool, operatorID string) (UnsupportedLockDevice, error) {
	sn = NormalizeSN(sn)
	now := time.Now()
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	update := bson.M{
		"$set": bson.M{
			"opt_out":     optOut,
			"operator_id": strings.TrimSpace(operatorID),
			"updated_at":  now,
		},
	}
	var out UnsupportedLockDevice
	err := t.UnsupportedLockDevices().FindOneAndUpdate(ctx, bson.M{"sn": sn}, update, opts).Decode(&out)
	return out, err
}

func (t *TenantDB) DeleteUnsupportedLockDevice(ctx context.Context, sn string) error {
	_, err := t.UnsupportedLockDevices().DeleteOne(ctx, bson.M{"sn": NormalizeSN(sn)})
	return err
}

func (t *TenantDB) DeleteUnauthorizedDevices(ctx context.Context, sns []string) error {
	if len(sns) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(sns))
	for _, sn := range sns {
		normalized = append(normalized, NormalizeSN(sn))
	}
	_, err := t.UnauthorizedDevices().DeleteMany(ctx, bson.M{"sn": bson.M{"$in": normalized}})
	return err
}

// ListLockCommands returns a page of lock command attempts.
// pageNumber is 0-based; pageSize must be > 0 (caller clamps).
func (t *TenantDB) ListLockCommands(ctx context.Context, sn string, pageNumber, pageSize int64) ([]LockCommandAttempt, int64, error) {
	filter := bson.M{}
	if sn != "" {
		filter["device_sn"] = NormalizeSN(sn)
	}
	total, err := t.LockCommands().CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(pageNumber * pageSize).
		SetLimit(pageSize)
	cursor, err := t.LockCommands().Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	var commands []LockCommandAttempt
	err = cursor.All(ctx, &commands)
	if commands == nil {
		commands = []LockCommandAttempt{}
	}
	return commands, total, err
}

func (t *TenantDB) CreateLockAuditLog(ctx context.Context, l LockAuditLog) error {
	l.ID = primitive.NewObjectID()
	l.SN = NormalizeSN(l.SN)
	l.CreatedAt = time.Now()
	_, err := t.LockAuditLogs().InsertOne(ctx, l)
	return err
}

// ListLockAuditLogs returns a page of lock audit logs.
// pageNumber is 0-based; pageSize must be > 0 (caller clamps).
func (t *TenantDB) ListLockAuditLogs(ctx context.Context, sn string, pageNumber, pageSize int64) ([]LockAuditLog, int64, error) {
	filter := bson.M{}
	if sn != "" {
		filter["sn"] = NormalizeSN(sn)
	}
	total, err := t.LockAuditLogs().CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(pageNumber * pageSize).
		SetLimit(pageSize)
	cursor, err := t.LockAuditLogs().Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	var logs []LockAuditLog
	err = cursor.All(ctx, &logs)
	if logs == nil {
		logs = []LockAuditLog{}
	}
	return logs, total, err
}

// lockCommandHistoryClearFilter matches completed attempts only.
// Pending/retry rows must remain for StartLockRetryScheduler.
func lockCommandHistoryClearFilter() bson.M {
	return bson.M{
		"status": bson.M{"$in": []LockCommandStatus{LockCommandSuccess, LockCommandFailed}},
	}
}

// ClearLockCommands removes completed lock command attempts (success/failed)
// for this tenant. Pending and retry rows are preserved so the retry scheduler
// can still resend in-flight lock/unlock commands.
func (t *TenantDB) ClearLockCommands(ctx context.Context) (int64, error) {
	res, err := t.LockCommands().DeleteMany(ctx, lockCommandHistoryClearFilter())
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// ClearLockAuditLogs removes all lock audit log entries for this tenant.
func (t *TenantDB) ClearLockAuditLogs(ctx context.Context) (int64, error) {
	res, err := t.LockAuditLogs().DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
