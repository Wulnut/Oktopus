package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	upgradeStatusPending     = "pending"
	upgradeStatusDownloading = "downloading"
	upgradeStatusSuccess     = "success"
	upgradeStatusFailed      = "failed"
)

type FirmwareUpgradeLog struct {
	ID               primitive.ObjectID `bson:"_id,omitempty"               json:"id"`
	DeviceSN         string             `bson:"device_sn"                   json:"device_sn"`
	DeviceAlias      string             `bson:"device_alias"                json:"device_alias"`
	CampaignID       primitive.ObjectID `bson:"campaign_id,omitempty"       json:"campaign_id,omitempty"`
	FirmwareID       primitive.ObjectID `bson:"firmware_id"                 json:"firmware_id"`
	FirmwareName     string             `bson:"firmware_name"               json:"firmware_name"`
	FirmwareBuildVer string             `bson:"firmware_build_ver"          json:"firmware_build_ver"`
	PreviousVersion  string             `bson:"previous_version"            json:"previous_version"`
	TriggerType      string             `bson:"trigger_type"                json:"trigger_type"`
	Status           string             `bson:"status"                      json:"status"`
	Error            string             `bson:"error,omitempty"             json:"error,omitempty"`
	RetryCount       int                `bson:"retry_count"                 json:"retry_count"`
	TriggeredAt      time.Time          `bson:"triggered_at"                json:"triggered_at"`
	CompletedAt      time.Time          `bson:"completed_at,omitempty"      json:"completed_at,omitempty"`
	ActiveAttemptKey string             `bson:"active_attempt_key,omitempty" json:"-"`
}

func isActiveUpgradeStatus(status string) bool {
	return status == upgradeStatusPending || status == upgradeStatusDownloading
}

func activeUpgradeAttemptKey(deviceSN string, firmwareID primitive.ObjectID) string {
	return deviceSN + ":" + firmwareID.Hex()
}

func (t *TenantDB) CreateUpgradeLog(ctx context.Context, l FirmwareUpgradeLog) (FirmwareUpgradeLog, error) {
	l.ID = primitive.NewObjectID()
	l.TriggeredAt = time.Now()
	if isActiveUpgradeStatus(l.Status) {
		l.ActiveAttemptKey = activeUpgradeAttemptKey(l.DeviceSN, l.FirmwareID)
	}
	_, err := t.UpgradeLogs().InsertOne(ctx, l)
	return l, err
}

func (t *TenantDB) UpdateUpgradeLogStatus(ctx context.Context, id primitive.ObjectID, status, errMsg string) error {
	update := bson.M{
		"status": status,
		"error":  errMsg,
	}
	updateDoc := bson.M{"$set": update}
	if status == upgradeStatusSuccess || status == upgradeStatusFailed {
		update["completed_at"] = time.Now()
		updateDoc["$unset"] = bson.M{"active_attempt_key": ""}
	}
	_, err := t.UpgradeLogs().UpdateOne(ctx,
		bson.M{"_id": id},
		updateDoc,
	)
	return err
}

func (t *TenantDB) GetLatestUpgradeLog(ctx context.Context, deviceSN string, firmwareID primitive.ObjectID) (FirmwareUpgradeLog, error) {
	var l FirmwareUpgradeLog
	opts := options.FindOne().SetSort(bson.D{{Key: "triggered_at", Value: -1}, {Key: "_id", Value: -1}})
	err := t.UpgradeLogs().FindOne(ctx, bson.M{
		"device_sn":   deviceSN,
		"firmware_id": firmwareID,
	}, opts).Decode(&l)
	return l, err
}

func (t *TenantDB) ListUpgradeLogsByCampaign(ctx context.Context, campaignID primitive.ObjectID, page, pageSize int64) ([]FirmwareUpgradeLog, int64, error) {
	filter := bson.M{"campaign_id": campaignID}

	total, err := t.UpgradeLogs().CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "triggered_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(page * pageSize).
		SetLimit(pageSize)

	cursor, err := t.UpgradeLogs().Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	var results []FirmwareUpgradeLog
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (t *TenantDB) ListUpgradeLogsByDevice(ctx context.Context, deviceSN string) ([]FirmwareUpgradeLog, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "triggered_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(50)

	cursor, err := t.UpgradeLogs().Find(ctx, bson.M{"device_sn": deviceSN}, opts)
	if err != nil {
		return nil, err
	}
	var results []FirmwareUpgradeLog
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (t *TenantDB) DeleteUpgradeLog(ctx context.Context, id primitive.ObjectID) error {
	_, err := t.UpgradeLogs().DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (t *TenantDB) GetPendingUpgradeLog(ctx context.Context, deviceSN string) (FirmwareUpgradeLog, error) {
	var l FirmwareUpgradeLog
	err := t.UpgradeLogs().FindOne(ctx, bson.M{
		"device_sn": deviceSN,
		"status":    bson.M{"$in": []string{"pending", "downloading"}},
	}).Decode(&l)
	return l, err
}

func (t *TenantDB) IncrementRetryAndResetStatus(ctx context.Context, id primitive.ObjectID) error {
	var existing FirmwareUpgradeLog
	if err := t.UpgradeLogs().FindOne(ctx, bson.M{"_id": id}).Decode(&existing); err != nil {
		return err
	}
	_, err := t.UpgradeLogs().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{
			"$inc": bson.M{"retry_count": 1},
			"$set": bson.M{
				"status":             upgradeStatusPending,
				"error":              "",
				"active_attempt_key": activeUpgradeAttemptKey(existing.DeviceSN, existing.FirmwareID),
			},
			"$unset": bson.M{"completed_at": ""},
		},
	)
	return err
}

// migrateActiveUpgradeAttempts backfills the unique key introduced for active
// firmware attempts. Newest records win; older duplicate active records are
// terminalized so existing tenants can be migrated without losing history.
func migrateActiveUpgradeAttempts(ctx context.Context, collection *mongo.Collection) error {
	filter := bson.M{
		"status":             bson.M{"$in": []string{upgradeStatusPending, upgradeStatusDownloading}},
		"active_attempt_key": bson.M{"$exists": false},
	}
	cursor, err := collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "triggered_at", Value: -1}, {Key: "_id", Value: -1}}))
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var attempt FirmwareUpgradeLog
		if err := cursor.Decode(&attempt); err != nil {
			return err
		}
		key := activeUpgradeAttemptKey(attempt.DeviceSN, attempt.FirmwareID)
		_, err := collection.UpdateOne(ctx,
			bson.M{"_id": attempt.ID, "active_attempt_key": bson.M{"$exists": false}},
			bson.M{"$set": bson.M{"active_attempt_key": key}},
		)
		if err == nil {
			continue
		}
		if !mongo.IsDuplicateKeyError(err) {
			return err
		}

		_, err = collection.UpdateOne(ctx, bson.M{"_id": attempt.ID}, bson.M{
			"$set": bson.M{
				"status":       upgradeStatusFailed,
				"error":        "superseded by another active attempt during index migration",
				"completed_at": time.Now(),
			},
			"$unset": bson.M{"active_attempt_key": ""},
		})
		if err != nil {
			return err
		}
	}
	return cursor.Err()
}
