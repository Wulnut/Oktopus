package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FirmwarePhase string

const (
	PhaseInternalTesting FirmwarePhase = "internal_testing"
	PhaseRelease         FirmwarePhase = "release"
)

type Firmware struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name         string             `bson:"name"          json:"name"`
	Vendor       string             `bson:"vendor"        json:"vendor"`
	Model        string             `bson:"model"         json:"model"`
	HWVersion    string             `bson:"hw_version"    json:"hw_version"`
	BuildVersion string             `bson:"build_version" json:"build_version"`
	FileSize     int64              `bson:"file_size"     json:"file_size"`
	Fingerprint  string             `bson:"fingerprint"   json:"fingerprint"`
	Phase        FirmwarePhase      `bson:"phase"         json:"phase"`
	DownloadURL  string             `bson:"download_url"  json:"download_url"`
	FileName     string             `bson:"file_name"     json:"file_name"`
	CreatedAt    time.Time          `bson:"created_at"    json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at"    json:"updated_at"`
}

func (t *TenantDB) ListFirmware(ctx context.Context) ([]Firmware, error) {
	cursor, err := t.Firmware().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var results []Firmware
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (t *TenantDB) CreateFirmware(ctx context.Context, fw Firmware) (Firmware, error) {
	fw.ID = primitive.NewObjectID()
	fw.CreatedAt = time.Now()
	fw.UpdatedAt = time.Now()
	_, err := t.Firmware().InsertOne(ctx, fw)
	return fw, err
}

func (t *TenantDB) DeleteFirmware(ctx context.Context, id primitive.ObjectID) error {
	_, err := t.Firmware().DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (t *TenantDB) GetFirmware(ctx context.Context, id primitive.ObjectID) (Firmware, error) {
	var fw Firmware
	err := t.Firmware().FindOne(ctx, bson.M{"_id": id}).Decode(&fw)
	return fw, err
}

func (t *TenantDB) UpdateFirmwarePhase(ctx context.Context, id primitive.ObjectID, phase FirmwarePhase) error {
	_, err := t.Firmware().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"phase": phase, "updated_at": time.Now()}})
	return err
}

func (t *TenantDB) UpdateFirmware(ctx context.Context, id primitive.ObjectID, fw Firmware) (int64, error) {
	result, err := t.Firmware().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"name":          fw.Name,
			"vendor":        fw.Vendor,
			"model":         fw.Model,
			"hw_version":    fw.HWVersion,
			"build_version": fw.BuildVersion,
			"updated_at":    time.Now(),
		}})
	if err != nil {
		return 0, err
	}
	return result.MatchedCount, nil
}
