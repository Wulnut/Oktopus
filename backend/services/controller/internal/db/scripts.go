package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ScriptVariable defines a variable placeholder used in script steps.
type ScriptVariable struct {
	Name        string `bson:"name"        json:"name"`
	Description string `bson:"description" json:"description"`
	Default     string `bson:"default"     json:"default"`
	Required    bool   `bson:"required"    json:"required"`
}

// ScriptCondition defines the condition for a CONDITION step.
type ScriptCondition struct {
	Source   string `bson:"source"   json:"source"`
	Path     string `bson:"path"     json:"path"`
	Param    string `bson:"param"    json:"param"`
	Operator string `bson:"operator" json:"operator"`
	Value    string `bson:"value"    json:"value"`
}

// ScriptParamSetting defines a parameter to set/add.
type ScriptParamSetting struct {
	Param    string `bson:"param"    json:"param"`
	Value    string `bson:"value"    json:"value"`
	Required bool   `bson:"required" json:"required"`
}

// ScriptInputArg defines an input argument for OPERATE.
type ScriptInputArg struct {
	Key   string `bson:"key"   json:"key"`
	Value string `bson:"value" json:"value"`
}

// ScriptStep represents a single step in a script.
type ScriptStep struct {
	ID            string               `bson:"id"                       json:"id"`
	Name          string               `bson:"name"                     json:"name"`
	Type          string               `bson:"type"                     json:"type"`
	OnError       string               `bson:"on_error,omitempty"       json:"on_error,omitempty"`
	ParamPaths    []string             `bson:"param_paths,omitempty"    json:"param_paths,omitempty"`
	MaxDepth      uint32               `bson:"max_depth,omitempty"      json:"max_depth,omitempty"`
	SaveResultAs  string               `bson:"save_result_as,omitempty" json:"save_result_as,omitempty"`
	ObjPath       string               `bson:"obj_path,omitempty"       json:"obj_path,omitempty"`
	ParamSettings []ScriptParamSetting `bson:"param_settings,omitempty" json:"param_settings,omitempty"`
	ObjPaths      []string             `bson:"obj_paths,omitempty"      json:"obj_paths,omitempty"`
	Command       string               `bson:"command,omitempty"        json:"command,omitempty"`
	CommandKey    string               `bson:"command_key,omitempty"    json:"command_key,omitempty"`
	InputArgs     []ScriptInputArg     `bson:"input_args,omitempty"     json:"input_args,omitempty"`
	Condition     *ScriptCondition     `bson:"condition,omitempty"      json:"condition,omitempty"`
	OnTrue        string               `bson:"on_true,omitempty"        json:"on_true,omitempty"`
	OnFalse       string               `bson:"on_false,omitempty"       json:"on_false,omitempty"`
	DurationMs    int                  `bson:"duration_ms,omitempty"    json:"duration_ms,omitempty"`
}

// Script is a saved sequence of USP commands.
type Script struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name        string             `bson:"name"          json:"name"`
	Description string             `bson:"description"   json:"description"`
	Tags        []string           `bson:"tags"          json:"tags"`
	Variables   []ScriptVariable   `bson:"variables"     json:"variables"`
	Steps       []ScriptStep       `bson:"steps"         json:"steps"`
	Builtin     bool               `bson:"builtin"       json:"builtin"`
	CreatedAt   time.Time          `bson:"created_at"    json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"    json:"updated_at"`
}

// StepResult records the outcome of a single step execution.
type StepResult struct {
	StepID          string      `bson:"step_id"                    json:"step_id"`
	StepName        string      `bson:"step_name"                  json:"step_name"`
	Status          string      `bson:"status"                     json:"status"`
	Response        interface{} `bson:"response,omitempty"         json:"response,omitempty"`
	Error           string      `bson:"error,omitempty"            json:"error,omitempty"`
	ConditionResult *bool       `bson:"condition_result,omitempty" json:"condition_result,omitempty"`
	JumpedTo        string      `bson:"jumped_to,omitempty"        json:"jumped_to,omitempty"`
	StartedAt       time.Time   `bson:"started_at"                 json:"started_at"`
	FinishedAt      time.Time   `bson:"finished_at"                json:"finished_at"`
}

// ScriptExecution records a single execution of a script against a device.
type ScriptExecution struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"  json:"id"`
	ScriptID    primitive.ObjectID `bson:"script_id"      json:"script_id"`
	ScriptName  string             `bson:"script_name"    json:"script_name"`
	DeviceSN    string             `bson:"device_sn"      json:"device_sn"`
	MTP         string             `bson:"mtp"            json:"mtp"`
	Status      string             `bson:"status"         json:"status"`
	Variables   map[string]string  `bson:"variables"      json:"variables"`
	StepResults []StepResult       `bson:"step_results"   json:"step_results"`
	StartedAt   time.Time          `bson:"started_at"     json:"started_at"`
	FinishedAt  time.Time          `bson:"finished_at"    json:"finished_at"`
	CreatedAt   time.Time          `bson:"created_at"     json:"created_at"`
}

// --- Script CRUD ---

func (t *TenantDB) ListScripts(ctx context.Context) ([]Script, error) {
	cursor, err := t.Scripts().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var results []Script
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (t *TenantDB) CreateScript(ctx context.Context, s Script) (Script, error) {
	s.ID = primitive.NewObjectID()
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	_, err := t.Scripts().InsertOne(ctx, s)
	return s, err
}

func (t *TenantDB) GetScript(ctx context.Context, id primitive.ObjectID) (Script, error) {
	var s Script
	err := t.Scripts().FindOne(ctx, bson.M{"_id": id}).Decode(&s)
	return s, err
}

func (t *TenantDB) UpdateScript(ctx context.Context, id primitive.ObjectID, s Script) error {
	_, err := t.Scripts().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"name":        s.Name,
			"description": s.Description,
			"tags":        s.Tags,
			"variables":   s.Variables,
			"steps":       s.Steps,
			"updated_at":  time.Now(),
		}})
	return err
}

func (t *TenantDB) DeleteScript(ctx context.Context, id primitive.ObjectID) error {
	_, err := t.Scripts().DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// --- Execution CRUD ---

func (t *TenantDB) CreateExecution(ctx context.Context, e ScriptExecution) (ScriptExecution, error) {
	e.ID = primitive.NewObjectID()
	e.CreatedAt = time.Now()
	_, err := t.ScriptExecs().InsertOne(ctx, e)
	return e, err
}

func (t *TenantDB) UpdateExecution(ctx context.Context, id primitive.ObjectID, e ScriptExecution) error {
	_, err := t.ScriptExecs().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"status":       e.Status,
			"step_results": e.StepResults,
			"finished_at":  e.FinishedAt,
		}})
	return err
}

func (t *TenantDB) ListExecutions(ctx context.Context, scriptID primitive.ObjectID) ([]ScriptExecution, error) {
	cursor, err := t.ScriptExecs().Find(ctx,
		bson.M{"script_id": scriptID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(50))
	if err != nil {
		return nil, err
	}
	var results []ScriptExecution
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (t *TenantDB) ListExecutionsByDevice(ctx context.Context, sn string) ([]ScriptExecution, error) {
	cursor, err := t.ScriptExecs().Find(ctx,
		bson.M{"device_sn": sn},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(50))
	if err != nil {
		return nil, err
	}
	var results []ScriptExecution
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (t *TenantDB) GetExecution(ctx context.Context, id primitive.ObjectID) (ScriptExecution, error) {
	var e ScriptExecution
	err := t.ScriptExecs().FindOne(ctx, bson.M{"_id": id}).Decode(&e)
	return e, err
}
