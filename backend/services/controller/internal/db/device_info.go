package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CachedDeviceInfo struct {
	DeviceSN  string    `bson:"device_sn"`
	InfoRaw   string    `bson:"info_raw"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func (d *Database) UpsertDeviceInfo(ctx context.Context, sn string, infoJSON []byte) error {
	_, err := d.deviceInfo.UpdateOne(ctx,
		bson.M{"device_sn": sn},
		bson.M{"$set": bson.M{
			"device_sn":  sn,
			"info_raw":   string(infoJSON),
			"updated_at": time.Now(),
		}},
		options.Update().SetUpsert(true))
	return err
}

func (d *Database) GetCachedDeviceInfo(ctx context.Context, sn string) (*CachedDeviceInfo, error) {
	var cached CachedDeviceInfo
	err := d.deviceInfo.FindOne(ctx, bson.M{"device_sn": sn}).Decode(&cached)
	if err != nil {
		return nil, err
	}
	return &cached, nil
}
