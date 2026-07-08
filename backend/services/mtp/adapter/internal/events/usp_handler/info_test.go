package usp_handler

import (
	"testing"

	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/db"
	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/usp/usp_msg"
	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/usp/usp_record"
	"google.golang.org/protobuf/proto"
)

// buildGetRespRecord builds a USP Record containing a GetResp with the given
// ReqPathResults, marshals it to bytes suitable for parseDeviceInfoMsg input.
func buildGetRespRecord(t *testing.T, reqPathResults []*usp_msg.GetResp_RequestedPathResult) []byte {
	t.Helper()

	getResp := &usp_msg.GetResp{
		ReqPathResults: reqPathResults,
	}

	msg := &usp_msg.Msg{
		Header: &usp_msg.Header{
			MsgId:   "test-msg-id",
			MsgType: usp_msg.Header_GET_RESP,
		},
		Body: &usp_msg.Body{
			MsgBody: &usp_msg.Body_Response{
				Response: &usp_msg.Response{
					RespType: &usp_msg.Response_GetResp{
						GetResp: getResp,
					},
				},
			},
		},
	}

	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal USP Msg: %v", err)
	}

	record := &usp_record.Record{
		Version:         "1.2",
		ToId:            "controller-id",
		FromId:          "device-id",
		PayloadSecurity: usp_record.Record_PLAINTEXT,
		RecordType: &usp_record.Record_NoSessionContext{
			NoSessionContext: &usp_record.NoSessionContextRecord{
				Payload: msgBytes,
			},
		},
	}

	recordBytes, err := proto.Marshal(record)
	if err != nil {
		t.Fatalf("Failed to marshal USP Record: %v", err)
	}

	return recordBytes
}

// makePathResult creates a single GetResp_RequestedPathResult with one
// ResolvedPathResult containing the given key-value pair.
func makePathResult(key, value string) *usp_msg.GetResp_RequestedPathResult {
	return &usp_msg.GetResp_RequestedPathResult{
		RequestedPath: "Device.DeviceInfo." + key,
		ResolvedPathResults: []*usp_msg.GetResp_ResolvedPathResult{
			{
				ResolvedPath: "Device.DeviceInfo.",
				ResultParams: map[string]string{key: value},
			},
		},
	}
}

func TestParseDeviceInfoMsg_FullResponse(t *testing.T) {
	// Build a valid full USP GetResp with all 6 ReqPathResults in the order
	// expected by parseDeviceInfoMsg:
	// [0] Manufacturer, [1] ModelName, [2] SoftwareVersion,
	// [3] SerialNumber (skipped), [4] ProductClass, [5] HardwareVersion
	results := []*usp_msg.GetResp_RequestedPathResult{
		makePathResult("Manufacturer", "TestVendor"),
		makePathResult("ModelName", "TestModel"),
		makePathResult("SoftwareVersion", "1.0.0"),
		makePathResult("SerialNumber", "SN12345"),
		makePathResult("ProductClass", "Router"),
		makePathResult("HardwareVersion", "rev-A"),
	}

	data := buildGetRespRecord(t, results)

	device, err := parseDeviceInfoMsg("SN12345", "test.subject", data, db.MQTT)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if device.SN != "SN12345" {
		t.Errorf("SN: got %q, want %q", device.SN, "SN12345")
	}
	if device.Vendor != "TestVendor" {
		t.Errorf("Vendor: got %q, want %q", device.Vendor, "TestVendor")
	}
	if device.Model != "TestModel" {
		t.Errorf("Model: got %q, want %q", device.Model, "TestModel")
	}
	if device.Version != "1.0.0" {
		t.Errorf("Version: got %q, want %q", device.Version, "1.0.0")
	}
	if device.ProductClass != "Router" {
		t.Errorf("ProductClass: got %q, want %q", device.ProductClass, "Router")
	}
	if device.HWVersion != "rev-A" {
		t.Errorf("HWVersion: got %q, want %q", device.HWVersion, "rev-A")
	}
	if device.Status != db.Online {
		t.Errorf("Status: got %d, want %d (Online)", device.Status, db.Online)
	}
	if device.Mqtt != db.Online {
		t.Errorf("Mqtt: got %d, want %d (Online) -- MTP was MQTT", device.Mqtt, db.Online)
	}
	if device.Websockets != db.Offline {
		t.Errorf("Websockets: got %d, want %d (Offline) -- MTP was not WS", device.Websockets, db.Offline)
	}
	if device.Stomp != db.Offline {
		t.Errorf("Stomp: got %d, want %d (Offline) -- MTP was not STOMP", device.Stomp, db.Offline)
	}
}

func TestParseDeviceInfoMsg_MTPWebsockets(t *testing.T) {
	results := []*usp_msg.GetResp_RequestedPathResult{
		makePathResult("Manufacturer", "WsVendor"),
		makePathResult("ModelName", "WsModel"),
		makePathResult("SoftwareVersion", "2.0.0"),
		makePathResult("SerialNumber", "WS001"),
		makePathResult("ProductClass", "Gateway"),
		makePathResult("HardwareVersion", "rev-B"),
	}

	data := buildGetRespRecord(t, results)

	device, err := parseDeviceInfoMsg("WS001", "test.subject", data, db.WEBSOCKETS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if device.Websockets != db.Online {
		t.Errorf("Websockets: got %d, want %d (Online)", device.Websockets, db.Online)
	}
	if device.Mqtt != db.Offline {
		t.Errorf("Mqtt: got %d, want %d (Offline)", device.Mqtt, db.Offline)
	}
}

func TestParseDeviceInfoMsg_MTPSTOMP(t *testing.T) {
	results := []*usp_msg.GetResp_RequestedPathResult{
		makePathResult("Manufacturer", "StompVendor"),
		makePathResult("ModelName", "StompModel"),
		makePathResult("SoftwareVersion", "3.0.0"),
		makePathResult("SerialNumber", "STOMP001"),
		makePathResult("ProductClass", "AP"),
		makePathResult("HardwareVersion", "rev-C"),
	}

	data := buildGetRespRecord(t, results)

	device, err := parseDeviceInfoMsg("STOMP001", "test.subject", data, db.STOMP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if device.Stomp != db.Online {
		t.Errorf("Stomp: got %d, want %d (Online)", device.Stomp, db.Online)
	}
	if device.Mqtt != db.Offline {
		t.Errorf("Mqtt: got %d, want %d (Offline)", device.Mqtt, db.Offline)
	}
}

func TestParseDeviceInfoMsg_PartialResponseRejected(t *testing.T) {
	results := []*usp_msg.GetResp_RequestedPathResult{
		makePathResult("Manufacturer", "PartialVendor"),
		makePathResult("ModelName", "PartialModel"),
		makePathResult("SoftwareVersion", "0.9.0"),
	}

	data := buildGetRespRecord(t, results)

	_, err := parseDeviceInfoMsg("PARTIAL001", "test.subject", data, db.MQTT)
	if err == nil {
		t.Fatal("expected error for partial ReqPathResults")
	}
}

func TestParseDeviceInfoMsg_EmptyResolvedPathResultsRejected(t *testing.T) {
	results := []*usp_msg.GetResp_RequestedPathResult{
		{
			RequestedPath:       "Device.DeviceInfo.Manufacturer",
			ResolvedPathResults: []*usp_msg.GetResp_ResolvedPathResult{},
		},
		makePathResult("ModelName", "TestModel"),
		makePathResult("SoftwareVersion", "1.0.0"),
		makePathResult("SerialNumber", "SN999"),
		makePathResult("ProductClass", "Router"),
		makePathResult("HardwareVersion", "rev-A"),
	}

	data := buildGetRespRecord(t, results)

	_, err := parseDeviceInfoMsg("SN999", "test.subject", data, db.MQTT)
	if err == nil {
		t.Fatal("expected error for empty ResolvedPathResults")
	}
}

func TestParseDeviceInfoMsg_InvalidProtobuf(t *testing.T) {
	data := []byte{0xFF, 0xFE, 0xAB, 0x00, 0x42}
	_, err := parseDeviceInfoMsg("BAD001", "test.subject", data, db.MQTT)
	if err == nil {
		t.Error("expected error for invalid protobuf data")
	}
}
