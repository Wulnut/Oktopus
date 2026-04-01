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

func (t *TenantDB) UpsertDeviceInfo(ctx context.Context, sn string, infoJSON []byte) error {
	_, err := t.DeviceInfo().UpdateOne(ctx,
		bson.M{"device_sn": sn},
		bson.M{"$set": bson.M{
			"device_sn":  sn,
			"info_raw":   string(infoJSON),
			"updated_at": time.Now(),
		}},
		options.Update().SetUpsert(true))
	return err
}

func (t *TenantDB) GetCachedDeviceInfo(ctx context.Context, sn string) (*CachedDeviceInfo, error) {
	var cached CachedDeviceInfo
	err := t.DeviceInfo().FindOne(ctx, bson.M{"device_sn": sn}).Decode(&cached)
	if err != nil {
		return nil, err
	}
	return &cached, nil
}
