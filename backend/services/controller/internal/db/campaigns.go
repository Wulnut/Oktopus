package db

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Campaign struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"      json:"id"`
	Vendor          string             `bson:"vendor"             json:"vendor"`
	Model           string             `bson:"model"              json:"model"`
	HWVersion       string             `bson:"hw_version"         json:"hw_version"`
	FirmwareID      primitive.ObjectID `bson:"firmware_id"        json:"firmware_id"`
	Concurrency     int                `bson:"concurrency"        json:"concurrency"`
	TimeWindowStart string             `bson:"time_window_start"  json:"time_window_start"`
	TimeWindowEnd   string             `bson:"time_window_end"    json:"time_window_end"`
	Enabled         bool               `bson:"enabled"            json:"enabled"`
	CreatedAt       time.Time          `bson:"created_at"         json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at"         json:"updated_at"`

	// Scheduled batch state (time-window auto batch).
	LastScheduledWindowKey  string    `bson:"last_scheduled_window_key,omitempty"  json:"last_scheduled_window_key,omitempty"`
	ScheduledBatchStatus    string    `bson:"scheduled_batch_status,omitempty"     json:"scheduled_batch_status,omitempty"`
	ScheduledBatchWindowKey string    `bson:"scheduled_batch_window_key,omitempty" json:"scheduled_batch_window_key,omitempty"`
	ScheduledBatchStartedAt time.Time `bson:"scheduled_batch_started_at,omitempty" json:"scheduled_batch_started_at,omitempty"`
}

const (
	ScheduledBatchInProgress = "in_progress"
	ScheduledBatchSuccess    = "success"
	ScheduledBatchFailed     = "failed"
	ScheduledBatchSkipped    = "skipped"
)

// IsScheduledBatchTerminal returns true when status indicates the window has been processed
// and the scheduler must not retry within the same window.
func IsScheduledBatchTerminal(status string) bool {
	switch status {
	case ScheduledBatchSuccess, ScheduledBatchFailed, ScheduledBatchSkipped:
		return true
	}
	return false
}

// ScheduledBatchLease is how long an in_progress scheduled batch lock is held before another instance may take over.
const ScheduledBatchLease = 10 * time.Minute

func (t *TenantDB) ListCampaigns(ctx context.Context) ([]Campaign, error) {
	cursor, err := t.Campaigns().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var results []Campaign
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (t *TenantDB) CreateCampaign(ctx context.Context, c Campaign) (Campaign, error) {
	// Normalize hardware fields. Combined with the case-insensitive collation
	// on the unique index, this guarantees consistent dedup semantics regardless
	// of how callers cased or padded the input.
	c.Vendor = strings.TrimSpace(c.Vendor)
	c.Model = strings.TrimSpace(c.Model)
	c.HWVersion = strings.TrimSpace(c.HWVersion)
	c.ID = primitive.NewObjectID()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	_, err := t.Campaigns().InsertOne(ctx, c)
	return c, err
}

func (t *TenantDB) GetCampaign(ctx context.Context, id primitive.ObjectID) (Campaign, error) {
	var c Campaign
	err := t.Campaigns().FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	return c, err
}

// GetCampaignByHardware looks up a campaign by (vendor, model, hw_version) using a
// case-insensitive indexed query (matching the campaigns_hardware_ci collation index).
// Inputs are TrimSpace'd to match the normalization performed on insert.
func (t *TenantDB) GetCampaignByHardware(ctx context.Context, vendor, model, hwVersion string) (Campaign, error) {
	var c Campaign
	opts := options.FindOne().SetCollation(&options.Collation{Locale: "en", Strength: 2})
	err := t.Campaigns().FindOne(ctx, bson.M{
		"vendor":     strings.TrimSpace(vendor),
		"model":      strings.TrimSpace(model),
		"hw_version": strings.TrimSpace(hwVersion),
	}, opts).Decode(&c)
	return c, err
}

func (t *TenantDB) UpdateCampaign(ctx context.Context, id primitive.ObjectID, c Campaign) (int64, error) {
	result, err := t.Campaigns().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"firmware_id":       c.FirmwareID,
			"concurrency":       c.Concurrency,
			"time_window_start": c.TimeWindowStart,
			"time_window_end":   c.TimeWindowEnd,
			"enabled":           c.Enabled,
			"updated_at":        time.Now(),
		}})
	if err != nil {
		return 0, err
	}
	return result.MatchedCount, nil
}

func (t *TenantDB) DeleteCampaign(ctx context.Context, id primitive.ObjectID) error {
	_, err := t.Campaigns().DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// TryAcquireScheduledBatch sets in_progress for windowKey when this window has not completed
// and no other holder has a fresh lease. Returns true if this caller won the lock.
func (t *TenantDB) TryAcquireScheduledBatch(ctx context.Context, campaignID primitive.ObjectID, windowKey string, now time.Time) (bool, error) {
	leaseCutoff := now.Add(-ScheduledBatchLease)
	filter := bson.M{
		"_id": campaignID,
		"last_scheduled_window_key": bson.M{"$ne": windowKey},
		"$or": []bson.M{
			{"scheduled_batch_status": bson.M{"$ne": ScheduledBatchInProgress}},
			{"scheduled_batch_window_key": bson.M{"$ne": windowKey}},
			{"scheduled_batch_started_at": bson.M{"$lt": leaseCutoff}},
		},
	}
	update := bson.M{"$set": bson.M{
		"scheduled_batch_status":     ScheduledBatchInProgress,
		"scheduled_batch_window_key": windowKey,
		"scheduled_batch_started_at": now,
	}}
	result, err := t.Campaigns().UpdateOne(ctx, filter, update)
	if err != nil {
		return false, err
	}
	return result.ModifiedCount == 1, nil
}

// CompleteScheduledBatch marks a scheduled window batch as terminal. status must be one of
// ScheduledBatchSuccess / ScheduledBatchFailed / ScheduledBatchSkipped. last_scheduled_window_key
// is set in every terminal case so a single window never reruns automatically.
func (t *TenantDB) CompleteScheduledBatch(ctx context.Context, campaignID primitive.ObjectID, windowKey, status string, now time.Time) error {
	if !IsScheduledBatchTerminal(status) {
		status = ScheduledBatchFailed
	}
	set := bson.M{
		"last_scheduled_window_key":  windowKey,
		"scheduled_batch_window_key": windowKey,
		"scheduled_batch_status":     status,
	}
	_, err := t.Campaigns().UpdateOne(ctx,
		bson.M{"_id": campaignID},
		bson.M{"$set": set},
	)
	return err
}

// DisableCampaignsByFirmware disables all campaigns that reference the given firmware ID.
func (t *TenantDB) DisableCampaignsByFirmware(ctx context.Context, firmwareID primitive.ObjectID) error {
	_, err := t.Campaigns().UpdateMany(ctx,
		bson.M{"firmware_id": firmwareID},
		bson.M{"$set": bson.M{"enabled": false, "updated_at": time.Now()}},
	)
	return err
}
