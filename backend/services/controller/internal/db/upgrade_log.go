package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
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
}

func (d *Database) CreateUpgradeLog(ctx context.Context, l FirmwareUpgradeLog) (FirmwareUpgradeLog, error) {
	l.ID = primitive.NewObjectID()
	l.TriggeredAt = time.Now()
	_, err := d.upgradeLogs.InsertOne(ctx, l)
	return l, err
}

func (d *Database) UpdateUpgradeLogStatus(ctx context.Context, id primitive.ObjectID, status, errMsg string) error {
	update := bson.M{
		"status": status,
		"error":  errMsg,
	}
	if status == "success" || status == "failed" {
		update["completed_at"] = time.Now()
	}
	_, err := d.upgradeLogs.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": update},
	)
	return err
}

func (d *Database) GetUpgradeLogByDeviceAndFirmware(ctx context.Context, deviceSN string, firmwareID primitive.ObjectID) (FirmwareUpgradeLog, error) {
	var l FirmwareUpgradeLog
	err := d.upgradeLogs.FindOne(ctx, bson.M{
		"device_sn":   deviceSN,
		"firmware_id": firmwareID,
	}).Decode(&l)
	return l, err
}

func (d *Database) ListUpgradeLogsByCampaign(ctx context.Context, campaignID primitive.ObjectID, page, pageSize int64) ([]FirmwareUpgradeLog, int64, error) {
	filter := bson.M{"campaign_id": campaignID}

	total, err := d.upgradeLogs.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "triggered_at", Value: -1}}).
		SetSkip(page * pageSize).
		SetLimit(pageSize)

	cursor, err := d.upgradeLogs.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	var results []FirmwareUpgradeLog
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (d *Database) ListUpgradeLogsByDevice(ctx context.Context, deviceSN string) ([]FirmwareUpgradeLog, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "triggered_at", Value: -1}}).
		SetLimit(50)

	cursor, err := d.upgradeLogs.Find(ctx, bson.M{"device_sn": deviceSN}, opts)
	if err != nil {
		return nil, err
	}
	var results []FirmwareUpgradeLog
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (d *Database) GetPendingUpgradeLog(ctx context.Context, deviceSN string) (FirmwareUpgradeLog, error) {
	var l FirmwareUpgradeLog
	err := d.upgradeLogs.FindOne(ctx, bson.M{
		"device_sn": deviceSN,
		"status":    bson.M{"$in": []string{"pending", "downloading"}},
	}).Decode(&l)
	return l, err
}

func (d *Database) IncrementRetryAndResetStatus(ctx context.Context, id primitive.ObjectID) error {
	_, err := d.upgradeLogs.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{
			"$inc": bson.M{"retry_count": 1},
			"$set": bson.M{"status": "pending", "error": ""},
		},
	)
	return err
}
