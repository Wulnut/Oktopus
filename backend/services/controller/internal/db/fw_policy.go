package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DeviceFWPolicy struct {
	ID               primitive.ObjectID `bson:"_id,omitempty"                    json:"id"`
	DeviceSN         string             `bson:"device_sn"                        json:"device_sn"`
	Policy           string             `bson:"policy"                           json:"policy"`
	ManualFirmwareID primitive.ObjectID `bson:"manual_firmware_id,omitempty"     json:"manual_firmware_id,omitempty"`
	UpdatedAt        time.Time          `bson:"updated_at"                       json:"updated_at"`
}

func (d *Database) GetDeviceFWPolicy(ctx context.Context, deviceSN string) (DeviceFWPolicy, error) {
	var p DeviceFWPolicy
	err := d.fwPolicies.FindOne(ctx, bson.M{"device_sn": deviceSN}).Decode(&p)
	if err == mongo.ErrNoDocuments {
		return DeviceFWPolicy{
			DeviceSN: deviceSN,
			Policy:   "campaign",
		}, nil
	}
	return p, err
}

func (d *Database) SetDeviceFWPolicy(ctx context.Context, p DeviceFWPolicy) error {
	p.UpdatedAt = time.Now()
	_, err := d.fwPolicies.UpdateOne(ctx,
		bson.M{"device_sn": p.DeviceSN},
		bson.M{"$set": p},
		options.Update().SetUpsert(true),
	)
	return err
}
