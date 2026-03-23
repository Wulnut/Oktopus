package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/db"
	local "github.com/leandrofars/oktopus/internal/nats"
	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_record"
	"github.com/leandrofars/oktopus/internal/usp/usp_utils"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/protobuf/proto"
)

// --- Mass Firmware Update ---

type massFirmwareRequest struct {
	FirmwareID  string   `json:"firmware_id"`
	DeviceSNs   []string `json:"device_sns"`
	Concurrency int      `json:"concurrency"`
}

const maxDevices = 500

func (a *Api) massFirmwareUpdate(w http.ResponseWriter, r *http.Request) {
	var req massFirmwareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.DeviceSNs) == 0 {
		http.Error(w, "device_sns is required", http.StatusBadRequest)
		return
	}
	if len(req.DeviceSNs) > maxDevices {
		http.Error(w, fmt.Sprintf("Too many devices: %d (max %d)", len(req.DeviceSNs), maxDevices), http.StatusBadRequest)
		return
	}

	fwID, err := primitive.ObjectIDFromHex(req.FirmwareID)
	if err != nil {
		http.Error(w, "Invalid firmware_id", http.StatusBadRequest)
		return
	}

	fw, err := a.db.GetFirmware(r.Context(), fwID)
	if err != nil {
		http.Error(w, "Firmware not found", http.StatusNotFound)
		return
	}
	if fw.DownloadURL == "" {
		http.Error(w, "Firmware has no download URL", http.StatusBadRequest)
		return
	}

	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	if concurrency > 20 {
		concurrency = 20
	}

	// Build initial device results
	deviceResults := make([]db.DeviceResult, len(req.DeviceSNs))
	for i, sn := range req.DeviceSNs {
		deviceResults[i] = db.DeviceResult{DeviceSN: sn, Status: "pending"}
	}

	ma := db.MassAction{
		Type:          db.MassActionFirmwareUpdate,
		Name:          fmt.Sprintf("FW Update: %s v%s", fw.Name, fw.BuildVersion),
		Status:        "running",
		DeviceSNs:     req.DeviceSNs,
		TotalDevices:  len(req.DeviceSNs),
		FirmwareID:    fw.ID,
		FirmwareName:  fmt.Sprintf("%s v%s", fw.Name, fw.BuildVersion),
		FirmwareURL:   fw.DownloadURL,
		DeviceResults: deviceResults,
		Concurrency:   concurrency,
		StartedAt:     time.Now(),
	}

	ma, err = a.db.CreateMassAction(r.Context(), ma)
	if err != nil {
		http.Error(w, "Failed to create mass action: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Launch background goroutine
	go a.runMassFirmwareUpdate(ma, fw)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(ma)
}

func (a *Api) runMassFirmwareUpdate(ma db.MassAction, fw db.Firmware) {
	sem := make(chan struct{}, ma.Concurrency)
	var wg sync.WaitGroup

	for i, dr := range ma.DeviceResults {
		// Check if cancelled
		current, err := a.db.GetMassAction(context.Background(), ma.ID)
		if err == nil && current.Status == "cancelled" {
			for j := i; j < len(ma.DeviceResults); j++ {
				if ma.DeviceResults[j].Status == "pending" {
					result := ma.DeviceResults[j]
					result.Status = "skipped"
					a.db.UpdateMassActionDevice(context.Background(), ma.ID, j, result)
				}
			}
			break
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, sn string) {
			defer wg.Done()
			defer func() { <-sem }()

			result := db.DeviceResult{
				DeviceSN:  sn,
				Status:    "running",
				StartedAt: time.Now(),
			}
			a.db.UpdateMassActionDevice(context.Background(), ma.ID, idx, result)

			mtp, online := deviceStateOKNoWrite(a.nc, sn)
			if !online {
				result.Status = "skipped"
				result.Error = "device offline"
				result.FinishedAt = time.Now()
				a.db.UpdateMassActionDevice(context.Background(), ma.ID, idx, result)
				a.db.IncrementMassActionProgress(context.Background(), ma.ID, false)
				return
			}

			result.MTP = mtp
			fwErr := performFirmwareUpdate(sn, mtp, fw, a.nc)

			success := fwErr == nil
			if fwErr != nil {
				result.Status = "failed"
				result.Error = fwErr.Error()
			} else {
				result.Status = "success"
			}
			result.FinishedAt = time.Now()
			a.db.UpdateMassActionDevice(context.Background(), ma.ID, idx, result)
			a.db.IncrementMassActionProgress(context.Background(), ma.ID, success)
		}(i, dr.DeviceSN)
	}

	wg.Wait()

	// Final status update — no concurrency at this point
	current, err := a.db.GetMassAction(context.Background(), ma.ID)
	if err != nil {
		log.Printf("runMassFirmwareUpdate: failed to read final state for %s: %v", ma.ID.Hex(), err)
		return
	}

	if current.Status == "cancelled" {
		// Already cancelled, just set finished_at
		a.db.UpdateMassAction(context.Background(), ma.ID, current)
		return
	}

	if current.FailureCount > 0 && current.SuccessCount == 0 {
		current.Status = "failed"
	} else {
		current.Status = "completed"
	}
	current.FinishedAt = time.Now()
	a.db.UpdateMassAction(context.Background(), ma.ID, current)
}

// performFirmwareUpdate executes firmware update on a single device without http.ResponseWriter.
func performFirmwareUpdate(sn, mtp string, fw db.Firmware, nc *nats.Conn) error {
	// Query firmware partitions
	getMsg := usp_utils.NewGetMsg(usp_msg.Get{
		ParamPaths: []string{"Device.DeviceInfo.FirmwareImage.*.Status"},
		MaxDepth:   1,
	})

	resp, err := sendUspMsgDirect(getMsg, sn, nc, mtp)
	if err != nil {
		return fmt.Errorf("failed to query firmware partitions: %w", err)
	}

	// Find available partition from the response
	partition := findAvailablePartition(resp)
	if partition == "" {
		return fmt.Errorf("no available firmware partition found")
	}

	log.Printf("Mass FW update: device=%s partition=%s url=%s", sn, partition, fw.DownloadURL)

	// Send Download() operate
	operateMsg := usp_utils.NewOperateMsg(usp_msg.Operate{
		Command:    "Device.DeviceInfo.FirmwareImage." + partition + "Download()",
		CommandKey: "Download()",
		SendResp:   true,
		InputArgs: map[string]string{
			"URL":          fw.DownloadURL,
			"AutoActivate": "true",
			"FileSize":     "0",
		},
	})

	protoMsg, err := proto.Marshal(&operateMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal operate message: %w", err)
	}

	record := usp_utils.NewUspRecord(protoMsg, sn)
	protoRecord, err := proto.Marshal(&record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	dummyW := &discardResponseWriter{}
	respData, err := bridge.NatsUspInteraction(
		local.DEVICE_SUBJECT_PREFIX+sn+".api",
		mtp+"-adapter.usp.v1."+sn+".api",
		protoRecord,
		dummyW,
		nc,
	)
	if err != nil {
		return fmt.Errorf("firmware download command failed: %w", err)
	}

	// Check USP-level error in response
	var receivedRecord usp_record.Record
	if err := proto.Unmarshal(respData, &receivedRecord); err != nil {
		return fmt.Errorf("failed to unmarshal firmware response record: %w", err)
	}
	var receivedMsg usp_msg.Msg
	if err := proto.Unmarshal(receivedRecord.GetNoSessionContext().Payload, &receivedMsg); err != nil {
		return fmt.Errorf("failed to unmarshal firmware response message: %w", err)
	}
	if errBody := receivedMsg.Body.GetError(); errBody != nil {
		return fmt.Errorf("device rejected firmware command: code=%d, message=%s", errBody.ErrCode, errBody.ErrMsg)
	}

	return nil
}

// findAvailablePartition finds an available firmware partition from a USP GetResp JSON.
func findAvailablePartition(data interface{}) string {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}

	reqPathResults, ok := dataMap["req_path_results"]
	if !ok {
		reqPathResults, ok = dataMap["reqPathResults"]
		if !ok {
			return ""
		}
	}

	results, ok := reqPathResults.([]interface{})
	if !ok {
		return ""
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
		if !ok || len(rrSlice) <= 1 {
			continue
		}
		for _, rr := range rrSlice {
			rrMap, ok := rr.(map[string]interface{})
			if !ok {
				continue
			}
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
			status, _ := paramsMap["Status"].(string)
			if status == "Available" || status == "ValidationFailed" {
				resolvedPath, _ := rrMap["resolved_path"].(string)
				if resolvedPath == "" {
					resolvedPath, _ = rrMap["resolvedPath"].(string)
				}
				// resolvedPath is like "Device.DeviceInfo.FirmwareImage.1." — split by "." and take last non-empty segment
				parts := strings.Split(strings.TrimSuffix(resolvedPath, "."), ".")
				if len(parts) > 0 {
					return parts[len(parts)-1]
				}
			}
		}
	}
	return ""
}

// --- Mass Script Execution ---

type massScriptRequest struct {
	ScriptID    string            `json:"script_id"`
	Variables   map[string]string `json:"variables"`
	DeviceSNs   []string          `json:"device_sns"`
	Concurrency int               `json:"concurrency"`
}

func (a *Api) massScriptExecution(w http.ResponseWriter, r *http.Request) {
	var req massScriptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.DeviceSNs) == 0 {
		http.Error(w, "device_sns is required", http.StatusBadRequest)
		return
	}
	if len(req.DeviceSNs) > maxDevices {
		http.Error(w, fmt.Sprintf("Too many devices: %d (max %d)", len(req.DeviceSNs), maxDevices), http.StatusBadRequest)
		return
	}

	scriptID, err := primitive.ObjectIDFromHex(req.ScriptID)
	if err != nil {
		http.Error(w, "Invalid script_id", http.StatusBadRequest)
		return
	}

	script, err := a.db.GetScript(r.Context(), scriptID)
	if err != nil {
		http.Error(w, "Script not found", http.StatusNotFound)
		return
	}

	// Merge variables with defaults
	if req.Variables == nil {
		req.Variables = make(map[string]string)
	}
	for _, v := range script.Variables {
		val, provided := req.Variables[v.Name]
		if !provided || val == "" {
			if v.Required && v.Default == "" {
				http.Error(w, fmt.Sprintf("Required variable '%s' not provided", v.Name), http.StatusBadRequest)
				return
			}
			if !provided {
				req.Variables[v.Name] = v.Default
			}
		}
	}

	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	if concurrency > 20 {
		concurrency = 20
	}

	deviceResults := make([]db.DeviceResult, len(req.DeviceSNs))
	for i, sn := range req.DeviceSNs {
		deviceResults[i] = db.DeviceResult{DeviceSN: sn, Status: "pending"}
	}

	ma := db.MassAction{
		Type:          db.MassActionScript,
		Name:          fmt.Sprintf("Script: %s", script.Name),
		Status:        "running",
		DeviceSNs:     req.DeviceSNs,
		TotalDevices:  len(req.DeviceSNs),
		ScriptID:      script.ID,
		ScriptName:    script.Name,
		Variables:     req.Variables,
		DeviceResults: deviceResults,
		Concurrency:   concurrency,
		StartedAt:     time.Now(),
	}

	ma, err = a.db.CreateMassAction(r.Context(), ma)
	if err != nil {
		http.Error(w, "Failed to create mass action: "+err.Error(), http.StatusInternalServerError)
		return
	}

	go a.runMassScriptExecution(ma, script, req.Variables)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(ma)
}

func (a *Api) runMassScriptExecution(ma db.MassAction, script db.Script, variables map[string]string) {
	sem := make(chan struct{}, ma.Concurrency)
	var wg sync.WaitGroup

	for i, dr := range ma.DeviceResults {
		current, err := a.db.GetMassAction(context.Background(), ma.ID)
		if err == nil && current.Status == "cancelled" {
			for j := i; j < len(ma.DeviceResults); j++ {
				if ma.DeviceResults[j].Status == "pending" {
					result := ma.DeviceResults[j]
					result.Status = "skipped"
					a.db.UpdateMassActionDevice(context.Background(), ma.ID, j, result)
				}
			}
			break
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, sn string) {
			defer wg.Done()
			defer func() { <-sem }()

			result := db.DeviceResult{
				DeviceSN:  sn,
				Status:    "running",
				StartedAt: time.Now(),
			}
			a.db.UpdateMassActionDevice(context.Background(), ma.ID, idx, result)

			mtp, online := deviceStateOKNoWrite(a.nc, sn)
			if !online {
				result.Status = "skipped"
				result.Error = "device offline"
				result.FinishedAt = time.Now()
				a.db.UpdateMassActionDevice(context.Background(), ma.ID, idx, result)
				a.db.IncrementMassActionProgress(context.Background(), ma.ID, false)
				return
			}

			result.MTP = mtp
			execution, execErr := a.executeScriptForDevice(script, sn, mtp, variables)

			success := true
			if execErr != nil || execution.Status == "failed" {
				result.Status = "failed"
				if execErr != nil {
					result.Error = execErr.Error()
				} else {
					result.Error = "script execution failed"
				}
				success = false
			} else {
				result.Status = "success"
			}
			result.ExecutionID = execution.ID
			result.FinishedAt = time.Now()
			a.db.UpdateMassActionDevice(context.Background(), ma.ID, idx, result)
			a.db.IncrementMassActionProgress(context.Background(), ma.ID, success)
		}(i, dr.DeviceSN)
	}

	wg.Wait()

	// Final status update — no concurrency at this point
	current, err := a.db.GetMassAction(context.Background(), ma.ID)
	if err != nil {
		log.Printf("runMassScriptExecution: failed to read final state for %s: %v", ma.ID.Hex(), err)
		return
	}

	if current.Status == "cancelled" {
		current.FinishedAt = time.Now()
		a.db.UpdateMassAction(context.Background(), ma.ID, current)
		return
	}

	if current.FailureCount > 0 && current.SuccessCount == 0 {
		current.Status = "failed"
	} else {
		current.Status = "completed"
	}
	current.FinishedAt = time.Now()
	a.db.UpdateMassAction(context.Background(), ma.ID, current)
}

// executeScriptForDevice runs a script on a single device and returns the execution record.
func (a *Api) executeScriptForDevice(script db.Script, sn, mtp string, variables map[string]string) (db.ScriptExecution, error) {
	execution := db.ScriptExecution{
		ScriptID:   script.ID,
		ScriptName: script.Name,
		DeviceSN:   sn,
		MTP:        mtp,
		Status:     "running",
		Variables:  variables,
		StartedAt:  time.Now(),
	}
	var err error
	execution, err = a.db.CreateExecution(context.Background(), execution)
	if err != nil {
		return execution, fmt.Errorf("failed to create execution log: %w", err)
	}

	savedResults := make(map[string]interface{})
	stepIndex := 0
	visitCounts := make(map[int]int)

	for stepIndex < len(script.Steps) {
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

		step := resolveStepVariables(script.Steps[stepIndex], variables)
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
				condResult, cerr := evaluateCondition(*step.Condition, savedResults)
				if cerr != nil {
					stepErr = cerr
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
			case len(onError) > 8 && onError[:8] == "skip_to:":
				target := onError[8:]
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
	a.db.UpdateExecution(context.Background(), execution.ID, execution)
	return execution, nil
}

// --- List / Get / Cancel ---

func (a *Api) listMassActions(w http.ResponseWriter, r *http.Request) {
	list, err := a.db.ListMassActions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []db.MassAction{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (a *Api) getMassAction(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	ma, err := a.db.GetMassAction(r.Context(), id)
	if err != nil {
		http.Error(w, "Mass action not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ma)
}

func (a *Api) cancelMassAction(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if err := a.db.CancelMassAction(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
