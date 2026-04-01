package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
	local "github.com/leandrofars/oktopus/internal/nats"
	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_record"
	"github.com/leandrofars/oktopus/internal/usp/usp_utils"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// --- Validation ---

var (
	pathRegex     = regexp.MustCompile(`^Device\.[\w.]+\.?$`)
	operateRegex  = regexp.MustCompile(`^Device\.[\w.]+\(\)$`)
	varNameRegex  = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	varRefRegex   = regexp.MustCompile(`\{\{([A-Za-z0-9_]+)\}\}`)
	validStepTypes = map[string]bool{
		"GET": true, "SET": true, "ADD": true, "DELETE": true,
		"OPERATE": true, "CONDITION": true, "DELAY": true,
	}
	validOperators = map[string]bool{
		"==": true, "!=": true, ">": true, "<": true,
		"contains": true, "exists": true,
	}
	maxSteps        = 50
	maxDelayMs      = 60000
	maxVarValueLen  = 1024
	maxSavedResults = 10
	maxBodySize     = int64(256 * 1024) // 256KB
)

func validateScript(s *db.Script) error {
	if s.Name == "" {
		return fmt.Errorf("script name is required")
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("script must have at least one step")
	}
	if len(s.Steps) > maxSteps {
		return fmt.Errorf("script cannot have more than %d steps", maxSteps)
	}

	// Validate variables
	for _, v := range s.Variables {
		if !varNameRegex.MatchString(v.Name) {
			return fmt.Errorf("invalid variable name: %s", v.Name)
		}
	}

	stepIDs := make(map[string]bool)
	savedResultNames := 0
	for _, step := range s.Steps {
		if step.ID == "" {
			return fmt.Errorf("step ID is required")
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("duplicate step ID: %s", step.ID)
		}
		stepIDs[step.ID] = true

		if !validStepTypes[step.Type] {
			return fmt.Errorf("invalid step type: %s", step.Type)
		}

		if step.SaveResultAs != "" {
			if !varNameRegex.MatchString(step.SaveResultAs) {
				return fmt.Errorf("invalid save_result_as name: %s", step.SaveResultAs)
			}
			savedResultNames++
			if savedResultNames > maxSavedResults {
				return fmt.Errorf("too many save_result_as entries (max %d)", maxSavedResults)
			}
		}

		// Validate on_error
		if step.OnError != "" && step.OnError != "abort" && step.OnError != "continue" {
			if !strings.HasPrefix(step.OnError, "skip_to:") {
				return fmt.Errorf("invalid on_error: %s", step.OnError)
			}
			target := strings.TrimPrefix(step.OnError, "skip_to:")
			if target == "" {
				return fmt.Errorf("skip_to target is empty")
			}
		}

		switch step.Type {
		case "GET":
			for _, p := range step.ParamPaths {
				if !pathRegex.MatchString(stripVars(p)) && !containsVar(p) {
					return fmt.Errorf("invalid param path: %s", p)
				}
			}
		case "SET", "ADD":
			if step.ObjPath != "" && !pathRegex.MatchString(stripVars(step.ObjPath)) && !containsVar(step.ObjPath) {
				return fmt.Errorf("invalid obj_path: %s", step.ObjPath)
			}
		case "DELETE":
			for _, p := range step.ObjPaths {
				if !pathRegex.MatchString(stripVars(p)) && !containsVar(p) {
					return fmt.Errorf("invalid obj_path: %s", p)
				}
			}
		case "OPERATE":
			if step.Command != "" && !operateRegex.MatchString(stripVars(step.Command)) && !containsVar(step.Command) {
				return fmt.Errorf("invalid operate command: %s", step.Command)
			}
		case "CONDITION":
			if step.Condition == nil {
				return fmt.Errorf("CONDITION step requires condition field")
			}
			if !validOperators[step.Condition.Operator] {
				return fmt.Errorf("invalid operator: %s", step.Condition.Operator)
			}
		case "DELAY":
			if step.DurationMs <= 0 || step.DurationMs > maxDelayMs {
				return fmt.Errorf("delay duration must be between 1 and %d ms", maxDelayMs)
			}
		}
	}

	// Validate jump targets exist
	for _, step := range s.Steps {
		if step.OnTrue != "" && !stepIDs[step.OnTrue] {
			return fmt.Errorf("on_true references unknown step: %s", step.OnTrue)
		}
		if step.OnFalse != "" && !stepIDs[step.OnFalse] {
			return fmt.Errorf("on_false references unknown step: %s", step.OnFalse)
		}
		if strings.HasPrefix(step.OnError, "skip_to:") {
			target := strings.TrimPrefix(step.OnError, "skip_to:")
			if !stepIDs[target] {
				return fmt.Errorf("skip_to references unknown step: %s", target)
			}
		}
	}

	return nil
}

func containsVar(s string) bool {
	return strings.Contains(s, "{{")
}

func stripVars(s string) string {
	return varRefRegex.ReplaceAllString(s, "X")
}

// --- CRUD Handlers ---

func (a *Api) listScripts(w http.ResponseWriter, r *http.Request) {
	list, err := a.tenantDB(r).ListScripts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []db.Script{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (a *Api) createScript(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var s db.Script
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateScript(&s); err != nil {
		http.Error(w, "Validation error: "+err.Error(), http.StatusBadRequest)
		return
	}
	created, err := a.tenantDB(r).CreateScript(r.Context(), s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (a *Api) getScript(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	s, err := a.tenantDB(r).GetScript(r.Context(), id)
	if err != nil {
		http.Error(w, "Script not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (a *Api) updateScript(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var s db.Script
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateScript(&s); err != nil {
		http.Error(w, "Validation error: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.tenantDB(r).UpdateScript(r.Context(), id, s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Api) deleteScript(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	// Check if builtin
	s, err := a.tenantDB(r).GetScript(r.Context(), id)
	if err != nil {
		http.Error(w, "Script not found", http.StatusNotFound)
		return
	}
	if s.Builtin {
		http.Error(w, "Cannot delete built-in scripts", http.StatusForbidden)
		return
	}
	if err := a.tenantDB(r).DeleteScript(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Execution History Handlers ---

func (a *Api) listScriptExecutions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	list, err := a.tenantDB(r).ListExecutions(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []db.ScriptExecution{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (a *Api) getExecution(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["execId"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	e, err := a.tenantDB(r).GetExecution(r.Context(), id)
	if err != nil {
		http.Error(w, "Execution not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(e)
}

// --- Script Execution Engine ---

// sendUspMsgDirect sends a USP message and returns the parsed response without writing to http.ResponseWriter.
func sendUspMsgDirect(msg usp_msg.Msg, sn string, nc *nats.Conn, mtp string) (interface{}, error) {
	protoMsg, err := proto.Marshal(&msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal USP message: %w", err)
	}

	record := usp_utils.NewUspRecord(protoMsg, sn)
	protoRecord, err := proto.Marshal(&record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal USP record: %w", err)
	}

	// Use a nil-safe ResponseWriter wrapper — NatsUspInteraction needs it for error writing
	dummyW := &discardResponseWriter{}
	data, err := bridge.NatsUspInteraction(
		local.DEVICE_SUBJECT_PREFIX+sn+".api",
		mtp+"-adapter.usp.v1."+sn+".api",
		protoRecord,
		dummyW,
		nc,
	)
	if err != nil {
		return nil, fmt.Errorf("NATS interaction failed: %w", err)
	}

	var receivedRecord usp_record.Record
	if err := proto.Unmarshal(data, &receivedRecord); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response record: %w", err)
	}
	var receivedMsg usp_msg.Msg
	if err := proto.Unmarshal(receivedRecord.GetNoSessionContext().Payload, &receivedMsg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response message: %w", err)
	}

	body := receivedMsg.Body.GetResponse()
	if body == nil {
		errorMsg := receivedMsg.Body.GetError()
		if errorMsg != nil {
			return nil, fmt.Errorf("USP error: code=%d, message=%s", errorMsg.ErrCode, errorMsg.ErrMsg)
		}
		return nil, fmt.Errorf("no response body")
	}

	// Convert protobuf response to JSON-serializable map
	var pbMsg proto.Message
	switch body.RespType.(type) {
	case *usp_msg.Response_GetResp:
		pbMsg = body.GetGetResp()
	case *usp_msg.Response_SetResp:
		pbMsg = body.GetSetResp()
	case *usp_msg.Response_AddResp:
		pbMsg = body.GetAddResp()
	case *usp_msg.Response_DeleteResp:
		pbMsg = body.GetDeleteResp()
	case *usp_msg.Response_OperateResp:
		pbMsg = body.GetOperateResp()
	default:
		return map[string]string{"result": "unknown response type"}, nil
	}

	jsonBytes, err := protojson.Marshal(pbMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}
	var result interface{}
	json.Unmarshal(jsonBytes, &result)
	return result, nil
}

// discardResponseWriter is a no-op ResponseWriter used when we don't want to write HTTP responses.
type discardResponseWriter struct {
	statusCode int
}

func (d *discardResponseWriter) Header() http.Header        { return http.Header{} }
func (d *discardResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (d *discardResponseWriter) WriteHeader(code int)        { d.statusCode = code }

// resolveVariables replaces {{VAR}} placeholders in a string.
func resolveVariables(s string, vars map[string]string) string {
	return varRefRegex.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-2]
		if val, ok := vars[name]; ok {
			return val
		}
		return match
	})
}

// resolveStepVariables resolves all variable placeholders in a step's fields.
func resolveStepVariables(step db.ScriptStep, vars map[string]string) db.ScriptStep {
	for i, p := range step.ParamPaths {
		step.ParamPaths[i] = resolveVariables(p, vars)
	}
	step.ObjPath = resolveVariables(step.ObjPath, vars)
	for i, p := range step.ObjPaths {
		step.ObjPaths[i] = resolveVariables(p, vars)
	}
	step.Command = resolveVariables(step.Command, vars)
	step.CommandKey = resolveVariables(step.CommandKey, vars)
	for i, ps := range step.ParamSettings {
		step.ParamSettings[i].Param = resolveVariables(ps.Param, vars)
		step.ParamSettings[i].Value = resolveVariables(ps.Value, vars)
	}
	for i, ia := range step.InputArgs {
		step.InputArgs[i].Key = resolveVariables(ia.Key, vars)
		step.InputArgs[i].Value = resolveVariables(ia.Value, vars)
	}
	if step.Condition != nil {
		c := *step.Condition
		c.Path = resolveVariables(c.Path, vars)
		c.Param = resolveVariables(c.Param, vars)
		c.Value = resolveVariables(c.Value, vars)
		step.Condition = &c
	}
	return step
}

// executeStepUSP runs a single USP step and returns the response.
func (a *Api) executeStepUSP(step db.ScriptStep, sn, mtp string) (interface{}, error) {
	var msg usp_msg.Msg

	switch step.Type {
	case "GET":
		msg = usp_utils.NewGetMsg(usp_msg.Get{
			ParamPaths: step.ParamPaths,
			MaxDepth:   step.MaxDepth,
		})
	case "SET":
		params := make([]*usp_msg.Set_UpdateParamSetting, len(step.ParamSettings))
		for i, ps := range step.ParamSettings {
			params[i] = &usp_msg.Set_UpdateParamSetting{
				Param:    ps.Param,
				Value:    ps.Value,
				Required: ps.Required,
			}
		}
		msg = usp_utils.NewSetMsg(usp_msg.Set{
			AllowPartial: true,
			UpdateObjs: []*usp_msg.Set_UpdateObject{{
				ObjPath:       step.ObjPath,
				ParamSettings: params,
			}},
		})
	case "ADD":
		params := make([]*usp_msg.Add_CreateParamSetting, len(step.ParamSettings))
		for i, ps := range step.ParamSettings {
			params[i] = &usp_msg.Add_CreateParamSetting{
				Param:    ps.Param,
				Value:    ps.Value,
				Required: ps.Required,
			}
		}
		msg = usp_utils.NewCreateMsg(usp_msg.Add{
			AllowPartial: true,
			CreateObjs: []*usp_msg.Add_CreateObject{{
				ObjPath:       step.ObjPath,
				ParamSettings: params,
			}},
		})
	case "DELETE":
		msg = usp_utils.NewDelMsg(usp_msg.Delete{
			AllowPartial: true,
			ObjPaths:     step.ObjPaths,
		})
	case "OPERATE":
		inputArgs := make(map[string]string)
		for _, ia := range step.InputArgs {
			inputArgs[ia.Key] = ia.Value
		}
		msg = usp_utils.NewOperateMsg(usp_msg.Operate{
			Command:    step.Command,
			CommandKey: step.CommandKey,
			SendResp:   true,
			InputArgs:  inputArgs,
		})
	default:
		return nil, fmt.Errorf("unsupported step type for USP: %s", step.Type)
	}

	return sendUspMsgDirect(msg, sn, a.nc, mtp)
}

// evaluateCondition checks a condition against saved results.
func evaluateCondition(cond db.ScriptCondition, savedResults map[string]interface{}) (bool, error) {
	resultData, ok := savedResults[cond.Source]
	if !ok {
		return false, fmt.Errorf("saved result '%s' not found", cond.Source)
	}

	// Navigate the response to find the parameter value
	paramValue := findParamValue(resultData, cond.Path, cond.Param)

	switch cond.Operator {
	case "exists":
		return paramValue != "", nil
	case "==":
		return paramValue == cond.Value, nil
	case "!=":
		return paramValue != cond.Value, nil
	case "contains":
		return strings.Contains(paramValue, cond.Value), nil
	case ">":
		a, errA := strconv.ParseFloat(paramValue, 64)
		b, errB := strconv.ParseFloat(cond.Value, 64)
		if errA != nil || errB != nil {
			return paramValue > cond.Value, nil // string comparison fallback
		}
		return a > b, nil
	case "<":
		a, errA := strconv.ParseFloat(paramValue, 64)
		b, errB := strconv.ParseFloat(cond.Value, 64)
		if errA != nil || errB != nil {
			return paramValue < cond.Value, nil
		}
		return a < b, nil
	default:
		return false, fmt.Errorf("unknown operator: %s", cond.Operator)
	}
}

// findParamValue searches a USP GetResp JSON structure for a parameter value.
func findParamValue(data interface{}, path, param string) string {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}

	// Navigate req_path_results -> resolved_path_results -> result_params
	reqPathResults, ok := dataMap["req_path_results"]
	if !ok {
		// Try camelCase variant from protojson
		reqPathResults, ok = dataMap["reqPathResults"]
		if !ok {
			return ""
		}
	}

	results, ok := reqPathResults.([]interface{})
	if !ok {
		return ""
	}

	normalizedPath := path
	if !strings.HasSuffix(normalizedPath, ".") {
		normalizedPath += "."
	}

	for _, r := range results {
		rMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		resolvedResults, ok := rMap["resolved_path_results"]
		if !ok {
			resolvedResults, ok = rMap["resolvedPathResults"]
			if !ok {
				continue
			}
		}
		rrSlice, ok := resolvedResults.([]interface{})
		if !ok {
			continue
		}
		for _, rr := range rrSlice {
			rrMap, ok := rr.(map[string]interface{})
			if !ok {
				continue
			}
			resolvedPath, _ := rrMap["resolved_path"].(string)
			if resolvedPath == "" {
				resolvedPath, _ = rrMap["resolvedPath"].(string)
			}
			if resolvedPath == normalizedPath {
				resultParams, ok := rrMap["result_params"]
				if !ok {
					resultParams, ok = rrMap["resultParams"]
					if !ok {
						continue
					}
				}
				paramsMap, ok := resultParams.(map[string]interface{})
				if !ok {
					continue
				}
				if val, ok := paramsMap[param]; ok {
					return fmt.Sprintf("%v", val)
				}
			}
		}
	}
	return ""
}

// findStepIndex returns the index of a step by ID.
func findStepIndex(steps []db.ScriptStep, id string) int {
	for i, s := range steps {
		if s.ID == id {
			return i
		}
	}
	return -1
}

func (a *Api) executeScriptHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid script ID", http.StatusBadRequest)
		return
	}
	sn := vars["sn"]
	mtp := vars["mtp"]

	// Load script
	script, err := a.tenantDB(r).GetScript(r.Context(), id)
	if err != nil {
		http.Error(w, "Script not found", http.StatusNotFound)
		return
	}

	// Parse variables from request body
	var reqBody struct {
		Variables map[string]string `json:"variables"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&reqBody)
	}
	if reqBody.Variables == nil {
		reqBody.Variables = make(map[string]string)
	}

	// Validate required variables
	for _, v := range script.Variables {
		val, provided := reqBody.Variables[v.Name]
		if !provided || val == "" {
			if v.Required && v.Default == "" {
				http.Error(w, fmt.Sprintf("Required variable '%s' not provided", v.Name), http.StatusBadRequest)
				return
			}
			if !provided {
				reqBody.Variables[v.Name] = v.Default
			}
		}
		// Validate variable value length
		if len(reqBody.Variables[v.Name]) > maxVarValueLen {
			http.Error(w, fmt.Sprintf("Variable '%s' exceeds max length", v.Name), http.StatusBadRequest)
			return
		}
	}

	// Check device is online
	if mtp == "" || mtp == "any" {
		mtp, _ = deviceStateOKNoWrite(a.nc, sn)
		if mtp == "" {
			http.Error(w, "Device is offline or not found", http.StatusServiceUnavailable)
			return
		}
	}

	// Create execution record
	execution := db.ScriptExecution{
		ScriptID:   script.ID,
		ScriptName: script.Name,
		DeviceSN:   sn,
		MTP:        mtp,
		Status:     "running",
		Variables:  reqBody.Variables,
		StartedAt:  time.Now(),
	}
	execution, err = a.tenantDB(r).CreateExecution(r.Context(), execution)
	if err != nil {
		http.Error(w, "Failed to create execution log: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Run steps
	savedResults := make(map[string]interface{})
	stepIndex := 0
	visitCounts := make(map[int]int)
	const maxTotalIterations = 500
	totalIterations := 0

	for stepIndex < len(script.Steps) {
		// Hard cap on total iterations
		totalIterations++
		if totalIterations > maxTotalIterations {
			execution.Status = "failed"
			execution.StepResults = append(execution.StepResults, db.StepResult{
				StepID:   script.Steps[stepIndex].ID,
				StepName: script.Steps[stepIndex].Name,
				Status:   "failed",
				Error:    fmt.Sprintf("exceeded maximum total iterations (%d)", maxTotalIterations),
			})
			break
		}

		// Loop detection
		visitCounts[stepIndex]++
		if visitCounts[stepIndex] > len(script.Steps) {
			execution.Status = "failed"
			execution.StepResults = append(execution.StepResults, db.StepResult{
				StepID:   script.Steps[stepIndex].ID,
				StepName: script.Steps[stepIndex].Name,
				Status:   "failed",
				Error:    "infinite loop detected",
			})
			break
		}

		step := resolveStepVariables(script.Steps[stepIndex], reqBody.Variables)
		stepResult := db.StepResult{
			StepID:    step.ID,
			StepName:  step.Name,
			StartedAt: time.Now(),
		}

		var stepErr error
		var response interface{}

		switch step.Type {
		case "CONDITION":
			if step.Condition == nil {
				stepErr = fmt.Errorf("missing condition")
			} else {
				condResult, err := evaluateCondition(*step.Condition, savedResults)
				if err != nil {
					stepErr = err
				} else {
					stepResult.ConditionResult = &condResult
					target := step.OnFalse
					if condResult {
						target = step.OnTrue
					}
					stepResult.JumpedTo = target
					stepResult.Status = "success"
					stepResult.FinishedAt = time.Now()
					execution.StepResults = append(execution.StepResults, stepResult)

					if target != "" {
						idx := findStepIndex(script.Steps, target)
						if idx >= 0 {
							stepIndex = idx
							continue
						}
					}
					stepIndex++
					continue
				}
			}

		case "DELAY":
			time.Sleep(time.Duration(step.DurationMs) * time.Millisecond)
			stepResult.Status = "success"
			stepResult.FinishedAt = time.Now()
			execution.StepResults = append(execution.StepResults, stepResult)
			stepIndex++
			continue

		default:
			response, stepErr = a.executeStepUSP(step, sn, mtp)
		}

		stepResult.FinishedAt = time.Now()

		if stepErr != nil {
			stepResult.Status = "failed"
			stepResult.Error = stepErr.Error()
			execution.StepResults = append(execution.StepResults, stepResult)

			onError := step.OnError
			if onError == "" {
				onError = "abort"
			}

			switch {
			case onError == "abort":
				execution.Status = "failed"
				goto done
			case onError == "continue":
				stepIndex++
			case strings.HasPrefix(onError, "skip_to:"):
				target := strings.TrimPrefix(onError, "skip_to:")
				idx := findStepIndex(script.Steps, target)
				if idx >= 0 {
					stepIndex = idx
				} else {
					execution.Status = "failed"
					goto done
				}
			}
			continue
		}

		stepResult.Status = "success"
		stepResult.Response = response
		execution.StepResults = append(execution.StepResults, stepResult)

		if step.SaveResultAs != "" {
			savedResults[step.SaveResultAs] = response
		}
		stepIndex++
	}

	if execution.Status != "failed" {
		execution.Status = "completed"
	}

done:
	execution.FinishedAt = time.Now()
	if err := a.tenantDB(r).UpdateExecution(r.Context(), execution.ID, execution); err != nil {
		log.Printf("failed to update execution %s: %v", execution.ID.Hex(), err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(execution)
}

// deviceStateOKNoWrite checks device state without writing to http.ResponseWriter.
// Returns the available MTP protocol name and true if the device is online.
func deviceStateOKNoWrite(nc *nats.Conn, sn string) (string, bool) {
	msg, err := bridge.NatsReqWithoutHttpSet[entity.Device](
		local.NATS_ADAPTER_SUBJECT+sn+".device",
		[]byte(""),
		nc,
	)
	if err != nil || msg == nil {
		return "", false
	}

	device := msg.Msg
	if device.Status != entity.Online {
		return "", false
	}

	if device.Mqtt == entity.Online {
		return entity.Mqtt, true
	}
	if device.Websockets == entity.Online {
		return entity.Websockets, true
	}
	if device.Stomp == entity.Online {
		return entity.Stomp, true
	}

	return "", false
}
