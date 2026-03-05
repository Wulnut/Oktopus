package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DeviceMetrics struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	DeviceSerial string             `bson:"device_serial" json:"device_serial"`
	Timestamp    time.Time          `bson:"timestamp"     json:"timestamp"`
	CPUUsage     float64            `bson:"cpu_usage"     json:"cpu_usage"`     // percent 0-100
	MemFree      int64              `bson:"mem_free"      json:"mem_free"`       // KB
	MemTotal     int64              `bson:"mem_total"     json:"mem_total"`      // KB
	StorageFree  int64              `bson:"storage_free"  json:"storage_free"`  // KB
	StorageUsed  int64              `bson:"storage_used"  json:"storage_used"`  // KB
	StorageTotal int64              `bson:"storage_total" json:"storage_total"` // KB
}

func (d *Database) StoreDeviceMetrics(ctx context.Context, m DeviceMetrics) error {
	m.ID = primitive.NewObjectID()
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}
	_, err := d.metrics.InsertOne(ctx, m)
	return err
}

func (d *Database) GetDeviceMetricsHistory(ctx context.Context, serial string, since time.Time) ([]DeviceMetrics, error) {
	filter := bson.M{
		"device_serial": serial,
		"timestamp":     bson.M{"$gte": since},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: 1}}).
		SetLimit(500)
	cursor, err := d.metrics.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var results []DeviceMetrics
	cursor.All(ctx, &results)
	return results, nil
}

func (d *Database) GetLatestDeviceMetrics(ctx context.Context, serial string) (DeviceMetrics, error) {
	var m DeviceMetrics
	opts := options.FindOne().SetSort(bson.D{{Key: "timestamp", Value: -1}})
	err := d.metrics.FindOne(ctx, bson.M{"device_serial": serial}, opts).Decode(&m)
	return m, err
}
