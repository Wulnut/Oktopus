package db

import (
	"context"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Database struct {
	client  *mongo.Client
	users   *mongo.Collection
	tenants *mongo.Collection
	ctx     context.Context
}

// Client returns the underlying Mongo client (e.g. for test fixtures sharing the controller pool).
func (d Database) Client() *mongo.Client {
	return d.client
}

// Disconnect closes the underlying Mongo client. Safe to call multiple times.
func (d Database) Disconnect(ctx context.Context) error {
	if d.client == nil {
		return nil
	}
	return d.client.Disconnect(ctx)
}

func NewDatabase(ctx context.Context, mongoUri string) Database {
	clientOptions := options.Client().ApplyURI(mongoUri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal(err)
	}
	return newDatabaseFromClient(ctx, client, mongoUri)
}

// NewDatabaseFromClient wires account-mngr collections on an existing client.
// Snapshot tests share one pool across all cases; production uses NewDatabase.
func NewDatabaseFromClient(ctx context.Context, client *mongo.Client, mongoURI string) Database {
	return newDatabaseFromClient(ctx, client, mongoURI)
}

// newDatabaseFromClient wires account-mngr collections on an existing client.
func newDatabaseFromClient(ctx context.Context, client *mongo.Client, mongoURI string) Database {
	var db Database
	db.client = client

	log.Println("Trying to ping Mongo database...")
	err := client.Ping(ctx, nil)
	if err != nil {
		log.Fatal("Couldn't connect to MongoDB --> ", err)
	}
	if mongoURI != "" {
		log.Println("Connected to MongoDB-->", mongoURI)
	}

	accountDB := client.Database("account-mngr")

	db.users = accountDB.Collection("users")
	_, err = db.users.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.M{"email": 1},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.tenants = accountDB.Collection("tenants")
	_, err = db.tenants.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "slug", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatalln(err)
	}

	db.ctx = ctx
	return db
}

// Ping checks MongoDB connectivity for readiness probes.
func (d *Database) Ping(ctx context.Context) error {
	if d.client == nil {
		return mongo.ErrClientDisconnected
	}
	return d.client.Ping(ctx, nil)
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
