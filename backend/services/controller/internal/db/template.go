package db

import (
	"context"
	"errors"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Template struct {
	Name  string `json:"name" bson:"name"`
	Type  string `json:"type" bson:"type"`
	Value string `json:"value" bson:"value"`
}

var ErrorTemplateExists = errors.New("message already exists")
var ErrorTemplateNotExists = errors.New("message don't exist")

func (t *TenantDB) FindTemplate(ctx context.Context, filter interface{}) (Template, error) {
	var result Template
	err := t.Templates().FindOne(ctx, filter).Decode(&result)
	return result, err
}

func (t *TenantDB) AllTemplates(ctx context.Context, filter interface{}) ([]Template, error) {
	var results []Template

	cursor, err := t.Templates().Find(ctx, filter)
	if err != nil {
		return results, err
	}
	if err = cursor.All(ctx, &results); err != nil {
		log.Println(err)
	}
	return results, err
}

func (t *TenantDB) AddTemplate(ctx context.Context, name, tr string, tmpl string) error {
	opts := options.FindOneAndReplace().SetUpsert(true)
	err := t.Templates().FindOneAndReplace(ctx, bson.D{{"name", name}}, Template{Name: name, Type: tr, Value: tmpl}, opts).Err()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("New message %s added to database", name)
			return nil
		}
		return err
	}
	log.Printf("Message %s already existed, and got replaced for new payload", name)
	return err
}

func (t *TenantDB) UpdateTemplate(ctx context.Context, name, tmpl string) error {
	result, err := t.Templates().UpdateOne(ctx, bson.D{{"name", name}}, bson.D{{"$set", bson.D{{"value", tmpl}}}})
	if err == nil {
		if result.MatchedCount == 0 {
			return ErrorTemplateNotExists
		}
	}
	return err
}

func (t *TenantDB) DeleteTemplate(ctx context.Context, name string) error {
	result, err := t.Templates().DeleteOne(ctx, bson.D{{"name", name}})
	if err == nil {
		if result.DeletedCount == 0 {
			return ErrorTemplateNotExists
		}
	}
	return err
}
