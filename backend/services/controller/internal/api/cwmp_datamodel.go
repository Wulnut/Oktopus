package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/cwmp"
	"github.com/leandrofars/oktopus/internal/entity"
	"github.com/leandrofars/oktopus/internal/utils"
	"github.com/nats-io/nats.go"
)

const (
	cwmpDataModelTR181 = "TR181"
	cwmpDataModelTR098 = "TR098"
	cwmpRootTR181      = "Device."
	cwmpRootTR098      = "InternetGatewayDevice."
)

type cwmpBrowseRequest struct {
	Path        string `json:"path"`
	FetchValues bool   `json:"fetch_values"`
}

type cwmpSetValuesRequest struct {
	Values map[string]string `json:"values"`
}

type cwmpParameterEntry struct {
	Name     string `json:"name"`
	Value    string `json:"value,omitempty"`
	Writable bool   `json:"writable"`
	IsObject bool   `json:"is_object"`
}

type cwmpBrowseResponse struct {
	Path       string               `json:"path"`
	DataModel  string               `json:"data_model"`
	Children   []cwmpParameterEntry `json:"children"`
	Parameters []cwmpParameterEntry `json:"parameters"`
}

type cwmpRootResponse struct {
	Path      string `json:"path"`
	DataModel string `json:"data_model"`
}

type cwmpInfoResponse struct {
	Manufacturer     string `json:"Manufacturer,omitempty"`
	ModelName        string `json:"ModelName,omitempty"`
	HardwareVersion  string `json:"HardwareVersion,omitempty"`
	SoftwareVersion  string `json:"SoftwareVersion,omitempty"`
	SerialNumber     string `json:"SerialNumber,omitempty"`
	ProductClass     string `json:"ProductClass,omitempty"`
	Description      string `json:"Description,omitempty"`
	Cached           bool   `json:"cached,omitempty"`
}

// quietResponseWriter captures HTTP status without writing to the client.
type quietResponseWriter struct {
	status int
	body   []byte
}

func (q *quietResponseWriter) Header() http.Header        { return make(http.Header) }
func (q *quietResponseWriter) Write(b []byte) (int, error) { q.body = append(q.body, b...); return len(b), nil }
func (q *quietResponseWriter) WriteHeader(code int)        { q.status = code }

// cwmpDeviceReady returns true when device is non-nil and CWMP-online; otherwise it
// writes an appropriate HTTP error and returns false. Guards against the rare
// case where getDeviceInfo upstream returned (nil, nil).
func cwmpDeviceReady(w http.ResponseWriter, device *entity.Device) bool {
	if device == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write(utils.Marshall("Device not found"))
		return false
	}
	if device.Cwmp != entity.Online {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write(utils.Marshall("CWMP device is offline"))
		return false
	}
	return true
}

func cwmpGetNames(sn, path string, nextLevel int, nc *nats.Conn, tenantSlug string) (cwmp.GetParameterNamesResponse, error) {
	qw := &quietResponseWriter{status: http.StatusOK}
	payload := cwmp.GetParameterNames(path, nextLevel)
	_, resp, err := cwmpInteraction[cwmp.GetParameterNamesResponse](sn, []byte(payload), qw, nc, tenantSlug)
	if err != nil {
		return resp, err
	}
	if qw.status != http.StatusOK {
		return resp, fmt.Errorf("%w: %s", errCwmpRequestFailed, strings.TrimSpace(string(qw.body)))
	}
	return resp, nil
}

func cwmpGetValues(sn string, names []string, nc *nats.Conn, tenantSlug string) (cwmp.GetParameterValuesResponse, error) {
	qw := &quietResponseWriter{status: http.StatusOK}
	payload := cwmp.GetParameterMultiValues(names)
	_, resp, err := cwmpInteraction[cwmp.GetParameterValuesResponse](sn, []byte(payload), qw, nc, tenantSlug)
	if err != nil {
		return resp, err
	}
	if qw.status != http.StatusOK {
		return resp, fmt.Errorf("%w: %s", errCwmpRequestFailed, strings.TrimSpace(string(qw.body)))
	}
	return resp, nil
}

func rootFromStoredDataModel(dataModel string) (path, model string, ok bool) {
	switch dataModel {
	case cwmpDataModelTR181:
		return cwmpRootTR181, cwmpDataModelTR181, true
	case cwmpDataModelTR098:
		return cwmpRootTR098, cwmpDataModelTR098, true
	default:
		return "", "", false
	}
}

func detectCwmpRootLive(sn string, nc *nats.Conn, tenantSlug string) (path, dataModel string, err error) {
	resp, err := cwmpGetNames(sn, cwmpRootTR181, 1, nc, tenantSlug)
	if err == nil && len(resp.ParameterList) > 0 {
		return cwmpRootTR181, cwmpDataModelTR181, nil
	}
	resp, err = cwmpGetNames(sn, cwmpRootTR098, 1, nc, tenantSlug)
	if err == nil && len(resp.ParameterList) > 0 {
		return cwmpRootTR098, cwmpDataModelTR098, nil
	}
	return "", "", errCwmpRootNotFound
}

func detectCwmpRoot(device *entity.Device, sn string, nc *nats.Conn, tenantSlug string) (path, dataModel string, err error) {
	if device != nil {
		if path, model, ok := rootFromStoredDataModel(device.DataModel); ok {
			return path, model, nil
		}
	}
	return detectCwmpRootLive(sn, nc, tenantSlug)
}

func dataModelForPath(path string) string {
	if strings.HasPrefix(path, cwmpRootTR098) {
		return cwmpDataModelTR098
	}
	if strings.HasPrefix(path, cwmpRootTR181) {
		return cwmpDataModelTR181
	}
	return ""
}

func cwmpDeviceInfoPrefix(dataModel string) string {
	if dataModel == cwmpDataModelTR098 {
		return cwmpRootTR098 + "DeviceInfo."
	}
	return cwmpRootTR181 + "DeviceInfo."
}

func cwmpInfoParamPaths(prefix string) []string {
	return []string{
		prefix + "Manufacturer",
		prefix + "ModelName",
		prefix + "HardwareVersion",
		prefix + "SoftwareVersion",
		prefix + "SerialNumber",
		prefix + "ProductClass",
		prefix + "Description",
	}
}

func cwmpAdapterInfoFallback(device *entity.Device) cwmpInfoResponse {
	info := cwmpInfoResponse{
		Manufacturer:    device.Vendor,
		ModelName:       device.Model,
		HardwareVersion: device.HWVersion,
		SoftwareVersion: device.Version,
		SerialNumber:    device.SN,
		ProductClass:    device.ProductClass,
		Cached:          true,
	}
	if info.ModelName == "" {
		info.ModelName = device.ProductClass
	}
	return info
}

func cwmpValuesToInfo(values cwmp.GetParameterValuesResponse) cwmpInfoResponse {
	info := cwmpInfoResponse{}
	for _, p := range values.ParameterList {
		key := paramLeafName(p.Name)
		switch key {
		case "Manufacturer":
			info.Manufacturer = p.Value
		case "ModelName":
			info.ModelName = p.Value
		case "HardwareVersion":
			info.HardwareVersion = p.Value
		case "SoftwareVersion":
			info.SoftwareVersion = p.Value
		case "SerialNumber":
			info.SerialNumber = p.Value
		case "ProductClass":
			info.ProductClass = p.Value
		case "Description":
			info.Description = p.Value
		}
	}
	return info
}

func paramLeafName(fullPath string) string {
	fullPath = strings.TrimSuffix(fullPath, ".")
	parts := strings.Split(fullPath, ".")
	if len(parts) == 0 {
		return fullPath
	}
	return parts[len(parts)-1]
}

func normalizeCwmpPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasSuffix(path, ".") {
		path += "."
	}
	return path
}

func splitBrowseEntries(list []cwmp.ParameterInfoStruct) (objects, params []cwmp.ParameterInfoStruct) {
	for _, item := range list {
		if strings.HasSuffix(item.Name, ".") {
			objects = append(objects, item)
		} else {
			params = append(params, item)
		}
	}
	return objects, params
}

var (
	errCwmpRequestFailed = errors.New("CWMP request failed")
	errCwmpRootNotFound  = errors.New("CWMP data model root not found")
)

// GET /api/tenants/{slug}/device/cwmp/{sn}/root
func (a *Api) cwmpRootGet(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	tenantSlug := middleware.GetTenantSlug(r)

	device, err := getDeviceInfo(w, sn, a.nc, tenantSlug)
	if err != nil {
		return
	}
	if !cwmpDeviceReady(w, device) {
		return
	}

	root, dataModel, err := detectCwmpRoot(device, sn, a.nc, tenantSlug)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write(utils.Marshall("Could not detect CWMP data model root"))
		return
	}

	utils.MarshallEncoder(cwmpRootResponse{Path: root, DataModel: dataModel}, w)
}

// GET /api/tenants/{slug}/device/cwmp/{sn}/info
func (a *Api) cwmpInfoGet(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	tenantSlug := middleware.GetTenantSlug(r)

	device, err := getDeviceInfo(w, sn, a.nc, tenantSlug)
	if err != nil {
		return
	}
	if device == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write(utils.Marshall("Device not found"))
		return
	}

	if device.Cwmp != entity.Online {
		if cached, err := a.tenantDB(r).GetCachedDeviceInfo(r.Context(), sn); err == nil {
			var info cwmpInfoResponse
			if json.Unmarshal([]byte(cached.InfoRaw), &info) == nil {
				info.Cached = true
				utils.MarshallEncoder(info, w)
				return
			}
		}
		utils.MarshallEncoder(cwmpAdapterInfoFallback(device), w)
		return
	}

	root, dataModel, err := detectCwmpRoot(device, sn, a.nc, tenantSlug)
	if err != nil {
		fallback := cwmpAdapterInfoFallback(device)
		fallback.Cached = true
		utils.MarshallEncoder(fallback, w)
		return
	}

	prefix := cwmpDeviceInfoPrefix(dataModel)
	_ = root

	values, err := cwmpGetValues(sn, cwmpInfoParamPaths(prefix), a.nc, tenantSlug)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write(utils.Marshall("Failed to read device info from CPE: " + err.Error()))
		return
	}

	info := cwmpValuesToInfo(values)
	if info.ModelName == "" && info.ProductClass == "" {
		fallback := cwmpAdapterInfoFallback(device)
		if info.Manufacturer == "" {
			info.Manufacturer = fallback.Manufacturer
		}
		if info.SerialNumber == "" {
			info.SerialNumber = fallback.SerialNumber
		}
		if info.SoftwareVersion == "" {
			info.SoftwareVersion = fallback.SoftwareVersion
		}
		if info.HardwareVersion == "" {
			info.HardwareVersion = fallback.HardwareVersion
		}
		if info.ModelName == "" {
			info.ModelName = fallback.ModelName
		}
		if info.ProductClass == "" {
			info.ProductClass = fallback.ProductClass
		}
	}

	if raw, err := json.Marshal(info); err == nil {
		tdb := a.tenantDB(r)
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := tdb.UpsertDeviceInfo(bgCtx, sn, raw); err != nil {
				log.Printf("cwmpInfoGet: cache error for %s: %v", sn, err)
			}
		}()
	}

	utils.MarshallEncoder(info, w)
}

// PUT /api/tenants/{slug}/device/cwmp/{sn}/parameters
func (a *Api) cwmpParametersBrowse(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	tenantSlug := middleware.GetTenantSlug(r)

	device, err := getDeviceInfo(w, sn, a.nc, tenantSlug)
	if err != nil {
		return
	}
	if !cwmpDeviceReady(w, device) {
		return
	}

	var req cwmpBrowseRequest
	utils.MarshallDecoder(&req, r.Body)
	path := normalizeCwmpPath(req.Path)
	dataModel := dataModelForPath(path)
	if dataModel == "" {
		if _, dm, ok := rootFromStoredDataModel(device.DataModel); ok {
			dataModel = dm
		}
	}
	if path == "" {
		root, dm, err := detectCwmpRoot(device, sn, a.nc, tenantSlug)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			w.Write(utils.Marshall("Could not detect CWMP data model root"))
			return
		}
		path = root
		dataModel = dm
	}

	namesResp, err := cwmpGetNames(sn, path, 1, a.nc, tenantSlug)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write(utils.Marshall("GetParameterNames failed: " + err.Error()))
		return
	}

	if dataModel == "" {
		dataModel = dataModelForPath(path)
	}

	objStructs, paramStructs := splitBrowseEntries(namesResp.ParameterList)

	children := make([]cwmpParameterEntry, 0, len(objStructs))
	for _, o := range objStructs {
		children = append(children, cwmpParameterEntry{
			Name:     o.Name,
			Writable: cwmp.ParamTypeIsWritable(o.Writable),
			IsObject: true,
		})
	}

	parameters := make([]cwmpParameterEntry, 0, len(paramStructs))
	for _, p := range paramStructs {
		parameters = append(parameters, cwmpParameterEntry{
			Name:     p.Name,
			Writable: cwmp.ParamTypeIsWritable(p.Writable),
			IsObject: false,
		})
	}

	if req.FetchValues && len(paramStructs) > 0 {
		names := make([]string, len(paramStructs))
		for i, p := range paramStructs {
			names[i] = p.Name
		}
		valuesResp, err := cwmpGetValues(sn, names, a.nc, tenantSlug)
		if err == nil {
			valueByName := make(map[string]string, len(valuesResp.ParameterList))
			for _, v := range valuesResp.ParameterList {
				valueByName[v.Name] = v.Value
			}
			for i := range parameters {
				parameters[i].Value = valueByName[parameters[i].Name]
			}
		}
	}

	utils.MarshallEncoder(cwmpBrowseResponse{
		Path:       path,
		DataModel:  dataModel,
		Children:   children,
		Parameters: parameters,
	}, w)
}

// PUT /api/tenants/{slug}/device/cwmp/{sn}/set
func (a *Api) cwmpSetValuesJson(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	tenantSlug := middleware.GetTenantSlug(r)

	device, err := getDeviceInfo(w, sn, a.nc, tenantSlug)
	if err != nil {
		return
	}
	if !cwmpDeviceReady(w, device) {
		return
	}

	var req cwmpSetValuesRequest
	utils.MarshallDecoder(&req, r.Body)
	if len(req.Values) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(utils.Marshall("No parameter values provided"))
		return
	}

	payload := cwmp.SetParameterMultiValues(req.Values)
	qw := &quietResponseWriter{status: http.StatusOK}
	data, _, err := cwmpInteraction[cwmp.SetParameterValuesResponse](sn, []byte(payload), qw, a.nc, tenantSlug)
	if err != nil {
		return
	}
	if qw.status != http.StatusOK {
		w.WriteHeader(qw.status)
		w.Write(qw.body)
		return
	}
	w.Write(data)
}
