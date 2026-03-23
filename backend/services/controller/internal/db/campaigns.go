package db

import (
	"context"
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
}

func (d *Database) ListCampaigns(ctx context.Context) ([]Campaign, error) {
	cursor, err := d.campaigns.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var results []Campaign
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (d *Database) CreateCampaign(ctx context.Context, c Campaign) (Campaign, error) {
	c.ID = primitive.NewObjectID()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	_, err := d.campaigns.InsertOne(ctx, c)
	return c, err
}

func (d *Database) GetCampaign(ctx context.Context, id primitive.ObjectID) (Campaign, error) {
	var c Campaign
	err := d.campaigns.FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	return c, err
}

func (d *Database) GetCampaignByHardware(ctx context.Context, vendor, model, hwVersion string) (Campaign, error) {
	var c Campaign
	err := d.campaigns.FindOne(ctx, bson.M{
		"vendor":     vendor,
		"model":      model,
		"hw_version": hwVersion,
	}).Decode(&c)
	return c, err
}

func (d *Database) UpdateCampaign(ctx context.Context, id primitive.ObjectID, c Campaign) (int64, error) {
	result, err := d.campaigns.UpdateOne(ctx,
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

func (d *Database) DeleteCampaign(ctx context.Context, id primitive.ObjectID) error {
	_, err := d.campaigns.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
