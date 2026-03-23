package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MassActionType string

const (
	MassActionFirmwareUpdate MassActionType = "firmware_update"
	MassActionScript         MassActionType = "script"
)

type DeviceResult struct {
	DeviceSN    string             `bson:"device_sn"              json:"device_sn"`
	Status      string             `bson:"status"                 json:"status"` // pending | running | success | failed | skipped
	MTP         string             `bson:"mtp"                    json:"mtp"`
	Error       string             `bson:"error,omitempty"        json:"error,omitempty"`
	ExecutionID primitive.ObjectID `bson:"execution_id,omitempty" json:"execution_id,omitempty"`
	StartedAt   time.Time          `bson:"started_at"             json:"started_at"`
	FinishedAt  time.Time          `bson:"finished_at"            json:"finished_at"`
}

type MassAction struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"          json:"id"`
	Type         MassActionType     `bson:"type"                   json:"type"`
	Name         string             `bson:"name"                   json:"name"`
	Status       string             `bson:"status"                 json:"status"` // pending | running | completed | failed | cancelled

	DeviceSNs    []string           `bson:"device_sns"             json:"device_sns"`
	TotalDevices int                `bson:"total_devices"          json:"total_devices"`

	// Firmware-specific
	FirmwareID   primitive.ObjectID `bson:"firmware_id,omitempty"  json:"firmware_id,omitempty"`
	FirmwareName string             `bson:"firmware_name,omitempty" json:"firmware_name,omitempty"`
	FirmwareURL  string             `bson:"firmware_url,omitempty" json:"firmware_url,omitempty"`

	// Script-specific
	ScriptID   primitive.ObjectID `bson:"script_id,omitempty"    json:"script_id,omitempty"`
	ScriptName string             `bson:"script_name,omitempty"  json:"script_name,omitempty"`
	Variables  map[string]string  `bson:"variables,omitempty"    json:"variables,omitempty"`

	// Execution tracking
	DeviceResults []DeviceResult `bson:"device_results" json:"device_results"`
	Progress      int            `bson:"progress"       json:"progress"`
	SuccessCount  int            `bson:"success_count"  json:"success_count"`
	FailureCount  int            `bson:"failure_count"  json:"failure_count"`
	Concurrency   int            `bson:"concurrency"    json:"concurrency"`

	CreatedAt  time.Time `bson:"created_at"             json:"created_at"`
	StartedAt  time.Time `bson:"started_at"             json:"started_at"`
	FinishedAt time.Time `bson:"finished_at"            json:"finished_at"`
}

func (d *Database) CreateMassAction(ctx context.Context, ma MassAction) (MassAction, error) {
	ma.ID = primitive.NewObjectID()
	ma.CreatedAt = time.Now()
	_, err := d.massActions.InsertOne(ctx, ma)
	return ma, err
}

func (d *Database) GetMassAction(ctx context.Context, id primitive.ObjectID) (MassAction, error) {
	var ma MassAction
	err := d.massActions.FindOne(ctx, bson.M{"_id": id}).Decode(&ma)
	return ma, err
}

func (d *Database) UpdateMassAction(ctx context.Context, id primitive.ObjectID, ma MassAction) error {
	_, err := d.massActions.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"status":         ma.Status,
			"device_results": ma.DeviceResults,
			"progress":       ma.Progress,
			"success_count":  ma.SuccessCount,
			"failure_count":  ma.FailureCount,
			"finished_at":    ma.FinishedAt,
		}})
	return err
}

func (d *Database) ListMassActions(ctx context.Context) ([]MassAction, error) {
	cursor, err := d.massActions.Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	var results []MassAction
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (d *Database) CancelMassAction(ctx context.Context, id primitive.ObjectID) error {
	_, err := d.massActions.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": "cancelled"}})
	return err
}
