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
	metrics        *mongo.Collection
	ctx            context.Context
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

