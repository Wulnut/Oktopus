package db

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Database struct {
	client         *mongo.Client
	users          *mongo.Collection
	template       *mongo.Collection
	firmware       *mongo.Collection
	messages       *mongo.Collection
	messagesErrors *mongo.Collection
	metrics          *mongo.Collection
	scripts          *mongo.Collection
	scriptExecutions *mongo.Collection
	massActions      *mongo.Collection
	deviceInfo       *mongo.Collection
	campaigns        *mongo.Collection
	fwPolicies       *mongo.Collection
	upgradeLogs      *mongo.Collection
	ctx              context.Context
}

func NewDatabase(ctx context.Context, mongoUri string) Database {
	var db Database

	clientOptions := options.Client().ApplyURI(mongoUri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	db.client = client

	log.Println("Trying to ping Mongo database...")
	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatal("Couldn't connect to MongoDB --> ", err)
	}

	log.Println("Connected to MongoDB-->", mongoUri)

	db.users = client.Database("account-mngr").Collection("users")
	indexField := bson.M{"email": 1}
	_, err = db.users.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    indexField,
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.template = client.Database("general").Collection("templates")
	indexField = bson.M{"name": 1}
	_, err = db.template.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    indexField,
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.firmware = client.Database("general").Collection("firmware")
	_, err = db.firmware.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatalln(err)
	}

	// Initialize messages collection
	db.messages = client.Database("usp").Collection("messages")
	err = createMessageIndexes(ctx, db.messages)
	if err != nil {
		log.Fatalln("Failed to create message indexes:", err)
	}

	// Initialize messages_errors collection
	db.messagesErrors = client.Database("usp").Collection("messages_errors")
	err = createMessageErrorIndexes(ctx, db.messagesErrors)
	if err != nil {
		log.Fatalln("Failed to create message error indexes:", err)
	}

	db.metrics = client.Database("usp").Collection("device_metrics")
	_, err = db.metrics.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "timestamp", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(604800), // 7 days
		},
		{
			Keys: bson.D{{Key: "device_serial", Value: 1}, {Key: "timestamp", Value: -1}},
		},
	})
	if err != nil {
		log.Fatalln("Failed to create metrics indexes:", err)
	}

	db.scripts = client.Database("general").Collection("scripts")
	_, err = db.scripts.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.scriptExecutions = client.Database("general").Collection("script_executions")
	_, err = db.scriptExecutions.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(2592000), // 30 days
		},
		{
			Keys: bson.D{{Key: "script_id", Value: 1}, {Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "device_sn", Value: 1}, {Key: "created_at", Value: -1}},
		},
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.massActions = client.Database("general").Collection("mass_actions")
	_, err = db.massActions.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(7776000), // 90 days
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}},
		},
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.deviceInfo = client.Database("general").Collection("device_info")
	_, err = db.deviceInfo.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "device_sn", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.campaigns = client.Database("general").Collection("campaigns")
	_, err = db.campaigns.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "vendor", Value: 1}, {Key: "model", Value: 1}, {Key: "hw_version", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.fwPolicies = client.Database("general").Collection("fw_policies")
	_, err = db.fwPolicies.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "device_sn", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.upgradeLogs = client.Database("general").Collection("upgrade_logs")
	_, err = db.upgradeLogs.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "campaign_id", Value: 1}, {Key: "triggered_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "device_sn", Value: 1}, {Key: "triggered_at", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "device_sn", Value: 1}, {Key: "firmware_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "device_sn", Value: 1}, {Key: "status", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "triggered_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(7776000), // 90 days
		},
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.ctx = ctx

	return db
}

// createMessageIndexes creates all indexes for the messages collection
func createMessageIndexes(ctx context.Context, collection *mongo.Collection) error {
	// Drop existing msg_id indexes if they exist (might be unique from previous version)
	// This handles the case where we changed from unique to non-unique
	// Try both auto-generated name and custom name
	_, _ = collection.Indexes().DropOne(ctx, "msg_id_1")
	_, _ = collection.Indexes().DropOne(ctx, "msg_id_idx")

	indexes := []mongo.IndexModel{
		// Primary query: device_serial + timestamp
		{
			Keys: bson.D{{Key: "device_serial", Value: 1}, {Key: "timestamp", Value: -1}},
		},
		// Index for message correlation (NOT unique - request/response share same msg_id)
		{
			Keys:    bson.D{{Key: "msg_id", Value: 1}},
			Options: options.Index().SetName("msg_id_idx"), // Custom name to avoid conflicts
		},
		// Compound index for filtered queries: device_serial + msg_type + timestamp
		{
			Keys: bson.D{{Key: "device_serial", Value: 1}, {Key: "msg_type", Value: 1}, {Key: "timestamp", Value: -1}},
		},
		// Compound index for direction filtering: device_serial + direction + timestamp
		{
			Keys: bson.D{{Key: "device_serial", Value: 1}, {Key: "direction", Value: 1}, {Key: "timestamp", Value: -1}},
		},
		// Compound index for source filtering: device_serial + source + timestamp
		{
			Keys: bson.D{{Key: "device_serial", Value: 1}, {Key: "source", Value: 1}, {Key: "timestamp", Value: -1}},
		},
		// Compound index for MTP filtering: device_serial + mtp + timestamp
		{
			Keys: bson.D{{Key: "device_serial", Value: 1}, {Key: "mtp", Value: 1}, {Key: "timestamp", Value: -1}},
		},
		// General history queries: timestamp descending
		{
			Keys: bson.D{{Key: "timestamp", Value: -1}},
		},
		// TTL index: timestamp ascending (90 days = 7776000 seconds)
		{
			Keys: bson.D{{Key: "timestamp", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(7776000), // 90 days
		},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexes)
	return err
}

// createMessageErrorIndexes creates all indexes for the messages_errors collection
func createMessageErrorIndexes(ctx context.Context, collection *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		// General error queries: timestamp descending
		{
			Keys: bson.D{{Key: "timestamp", Value: -1}},
		},
		// Filter by error type: error_type + timestamp
		{
			Keys: bson.D{{Key: "error_type", Value: 1}, {Key: "timestamp", Value: -1}},
		},
	}

	_, err := collection.Indexes().CreateMany(ctx, indexes)
	return err
}

