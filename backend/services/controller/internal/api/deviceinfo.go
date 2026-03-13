package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_utils"
)

func newCommandKey() string {
	return fmt.Sprintf("USP_%08X", rand.Uint32())
}

// GET /api/device/{sn}/{mtp}/info
// Returns DeviceInfo metadata via USP GET
func (a *Api) deviceInfoGet(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	mtp, err := getMtpFromRequest(r, w)
	if err != nil {
		return
	}
	if mtp == "" {
		var ok bool
		mtp, ok = deviceStateOK(w, a.nc, sn)
		if !ok {
			return
		}
	}
	msg := usp_utils.NewGetMsg(usp_msg.Get{
		ParamPaths: []string{
			"Device.DeviceInfo.Manufacturer",
			"Device.DeviceInfo.ModelName",
			"Device.DeviceInfo.HardwareVersion",
			"Device.DeviceInfo.SoftwareVersion",
			"Device.DeviceInfo.SerialNumber",
			"Device.DeviceInfo.ProductClass",
			"Device.DeviceInfo.Description",
		},
		MaxDepth: 1,
	})
	sendUspMsg(msg, sn, w, a.nc, mtp)
}

// GET /api/device/{sn}/{mtp}/wifi-usp
// Returns WiFi configuration for USP devices
func (a *Api) deviceWifiUspGet(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	mtp, err := getMtpFromRequest(r, w)
	if err != nil {
		return
	}
	if mtp == "" {
		var ok bool
		mtp, ok = deviceStateOK(w, a.nc, sn)
		if !ok {
			return
		}
	}
	msg := usp_utils.NewGetMsg(usp_msg.Get{
		ParamPaths: []string{
			"Device.WiFi.Radio.",
			"Device.WiFi.SSID.",
			"Device.WiFi.AccessPoint.",
		},
		MaxDepth: 3,
	})
	sendUspMsg(msg, sn, w, a.nc, mtp)
}

// GET /api/device/{sn}/{mtp}/interfaces
// Returns IP interface data (Ethernet IPv4/IPv6)
func (a *Api) deviceInterfacesGet(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	mtp, err := getMtpFromRequest(r, w)
	if err != nil {
		return
	}
	if mtp == "" {
		var ok bool
		mtp, ok = deviceStateOK(w, a.nc, sn)
		if !ok {
			return
		}
	}
	msg := usp_utils.NewGetMsg(usp_msg.Get{
		ParamPaths: []string{
			"Device.IP.Interface.",
		},
		MaxDepth: 3,
	})
	sendUspMsg(msg, sn, w, a.nc, mtp)
}

// responseRecorder captures the response body for dual-use (store + forward)
type responseRecorder struct {
	header     http.Header
	body       []byte
	statusCode int
}

func (rec *responseRecorder) Header() http.Header {
	return rec.header
}
func (rec *responseRecorder) WriteHeader(code int) {
	rec.statusCode = code
}
func (rec *responseRecorder) Write(b []byte) (int, error) {
	rec.body = append(rec.body, b...)
	return len(b), nil
}

// GET /api/device/{sn}/{mtp}/performance
// Returns current CPU/RAM/storage metrics, stores them, and returns the raw USP response
func (a *Api) devicePerformanceGet(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	mtp, err := getMtpFromRequest(r, w)
	if err != nil {
		return
	}
	if mtp == "" {
		var ok bool
		mtp, ok = deviceStateOK(w, a.nc, sn)
		if !ok {
			return
		}
	}
	msg := usp_utils.NewGetMsg(usp_msg.Get{
		ParamPaths: []string{
			"Device.DeviceInfo.ProcessStatus.CPUUsage",
			"Device.DeviceInfo.MemoryStatus.Free",
			"Device.DeviceInfo.MemoryStatus.Total",
			"Device.StorageService.1.LogicalVolume.",
		},
		MaxDepth: 2,
	})

	rec := &responseRecorder{header: make(http.Header), statusCode: http.StatusOK}
	sendUspMsg(msg, sn, rec, a.nc, mtp)

	if rec.statusCode == http.StatusOK {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		go func() {
			defer cancel()
			a.storePerformanceMetrics(bgCtx, sn, rec.body)
		}()
	}

	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.statusCode)
	w.Write(rec.body)
}

func (a *Api) storePerformanceMetrics(ctx context.Context, sn string, data []byte) {
	var resp usp_msg.GetResp
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("storePerformanceMetrics: unmarshal error for %s: %v", sn, err)
		return
	}
	m := db.DeviceMetrics{DeviceSerial: sn, Timestamp: time.Now()}
	for _, pathResult := range resp.ReqPathResults {
		for _, resolved := range pathResult.ResolvedPathResults {
			for k, v := range resolved.ResultParams {
				switch k {
				case "CPUUsage":
					if f, err := strconv.ParseFloat(v, 64); err == nil {
						m.CPUUsage = f
					}
				case "Free":
					if i, err := strconv.ParseInt(v, 10, 64); err == nil {
						m.MemFree = i
					}
				case "Total":
					if i, err := strconv.ParseInt(v, 10, 64); err == nil {
						m.MemTotal = i
					}
				}
			}
		}
	}
	if err := a.db.StoreDeviceMetrics(ctx, m); err != nil {
		log.Printf("storePerformanceMetrics: store error for %s: %v", sn, err)
	}
}

// GET /api/device/{sn}/metrics?since=24  (hours)
// Returns historical metrics from MongoDB
func (a *Api) deviceMetricsHistory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sn := vars["sn"]
	since := time.Now().Add(-24 * time.Hour)
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if hours, err := strconv.Atoi(sinceStr); err == nil {
			since = time.Now().Add(-time.Duration(hours) * time.Hour)
		}
	}
	metrics, err := a.db.GetDeviceMetricsHistory(r.Context(), sn, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if metrics == nil {
		metrics = []db.DeviceMetrics{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// PUT /api/device/{sn}/{mtp}/reboot
func (a *Api) deviceReboot(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	mtp, err := getMtpFromRequest(r, w)
	if err != nil {
		return
	}
	if mtp == "" {
		var ok bool
		mtp, ok = deviceStateOK(w, a.nc, sn)
		if !ok {
			return
		}
	}
	msg := usp_utils.NewOperateMsg(usp_msg.Operate{
		Command:    "Device.Reboot()",
		CommandKey: newCommandKey(),
		SendResp:   true,
		InputArgs: map[string]string{
			"Cause":  "RemoteReboot",
			"Reason": "Manual Reboot",
		},
	})
	sendUspMsg(msg, sn, w, a.nc, mtp)
}

// PUT /api/device/{sn}/{mtp}/factory-reset
func (a *Api) deviceFactoryReset(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	mtp, err := getMtpFromRequest(r, w)
	if err != nil {
		return
	}
	if mtp == "" {
		var ok bool
		mtp, ok = deviceStateOK(w, a.nc, sn)
		if !ok {
			return
		}
	}
	msg := usp_utils.NewOperateMsg(usp_msg.Operate{
		Command:    "Device.FactoryReset()",
		CommandKey: newCommandKey(),
		SendResp:   true,
	})
	sendUspMsg(msg, sn, w, a.nc, mtp)
}

// PUT /api/device/{sn}/{mtp}/restart-agent
func (a *Api) deviceRestartAgent(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	mtp, err := getMtpFromRequest(r, w)
	if err != nil {
		return
	}
	if mtp == "" {
		var ok bool
		mtp, ok = deviceStateOK(w, a.nc, sn)
		if !ok {
			return
		}
	}
	msg := usp_utils.NewOperateMsg(usp_msg.Operate{
		Command:    "Device.USPAgent.Restart()",
		CommandKey: newCommandKey(),
		SendResp:   true,
	})
	sendUspMsg(msg, sn, w, a.nc, mtp)
}
