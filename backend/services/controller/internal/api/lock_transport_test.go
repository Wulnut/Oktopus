package api

import (
	"testing"

	"github.com/leandrofars/oktopus/internal/entity"
)

func TestParseUspGetParamValue_ShortNameLikeDataModelUI(t *testing.T) {
	// DataModel UI stores result_params under the short parameter name
	// (e.g. "InternetWanIP"), not the full TR-181 path.
	raw := []byte(`{
		"req_path_results": [{
			"requested_path": "Device.X_TELKOMSEL_OntLock.InternetWanIP",
			"resolved_path_results": [{
				"resolved_path": "Device.X_TELKOMSEL_OntLock.",
				"result_params": {
					"InternetWanIP": "10.172.16.165"
				}
			}]
		}]
	}`)

	got, err := parseUspGetParamValue(raw, "Device.X_TELKOMSEL_OntLock.InternetWanIP")
	if err != nil {
		t.Fatalf("parseUspGetParamValue: %v", err)
	}
	if got != "10.172.16.165" {
		t.Fatalf("expected 10.172.16.165, got %q", got)
	}
}

func TestParseUspGetParamValue_FullPathKey(t *testing.T) {
	raw := []byte(`{
		"req_path_results": [{
			"resolved_path_results": [{
				"resolved_path": "Device.X_TELKOMSEL_OntLock.InternetWanIP",
				"result_params": {
					"Device.X_TELKOMSEL_OntLock.InternetWanIP": "10.0.0.1"
				}
			}]
		}]
	}`)

	got, err := parseUspGetParamValue(raw, "Device.X_TELKOMSEL_OntLock.InternetWanIP")
	if err != nil {
		t.Fatalf("parseUspGetParamValue: %v", err)
	}
	if got != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %q", got)
	}
}

func TestParseUspGetParamValue_ResolvedPathPlusShortName(t *testing.T) {
	raw := []byte(`{
		"req_path_results": [{
			"resolved_path_results": [{
				"resolved_path": "Device.X_TELKOMSEL_OntLock.",
				"result_params": {
					"Lock": "0"
				}
			}]
		}]
	}`)

	got, err := parseUspGetParamValue(raw, "Device.X_TELKOMSEL_OntLock.Lock")
	if err != nil {
		t.Fatalf("parseUspGetParamValue: %v", err)
	}
	if got != "0" {
		t.Fatalf("expected 0, got %q", got)
	}
}

func TestParseUspGetParamValue_NotFound(t *testing.T) {
	raw := []byte(`{
		"req_path_results": [{
			"resolved_path_results": [{
				"resolved_path": "Device.X_TELKOMSEL_OntLock.",
				"result_params": {
					"Lock": "0"
				}
			}]
		}]
	}`)

	_, err := parseUspGetParamValue(raw, "Device.X_TELKOMSEL_OntLock.InternetWanIP")
	if err == nil {
		t.Fatal("expected error when parameter is missing")
	}
}


func TestLockDeviceMTP_CwmpOnly(t *testing.T) {
	device := entity.Device{
		SN:   "SN-CWMP",
		Cwmp: entity.Online,
	}
	if mtp := lockDeviceMTP(device); mtp != "cwmp" {
		t.Fatalf("expected cwmp, got %s", mtp)
	}
}

func TestLockDeviceMTP_MqttOnly(t *testing.T) {
	device := entity.Device{
		SN:   "SN-MQTT",
		Mqtt: entity.Online,
	}
	if mtp := lockDeviceMTP(device); mtp != entity.Mqtt {
		t.Fatalf("expected mqtt, got %s", mtp)
	}
}

func TestLockDeviceMTP_MultiProtocol_PrefersCwmp(t *testing.T) {
	device := entity.Device{
		SN:   "SN-MULTI",
		Cwmp: entity.Online,
		Mqtt: entity.Online,
	}
	if mtp := lockDeviceMTP(device); mtp != "cwmp" {
		t.Fatalf("expected cwmp preferred over mqtt, got %s", mtp)
	}
}

func TestLockDeviceMTP_AllOffline(t *testing.T) {
	device := entity.Device{SN: "SN-OFFLINE"}
	if mtp := lockDeviceMTP(device); mtp != "" {
		t.Fatalf("expected empty string for offline device, got %s", mtp)
	}
}

func TestLockDeviceMTP_WsAndStomp(t *testing.T) {
	device := entity.Device{
		SN:         "SN-WS",
		Websockets: entity.Online,
	}
	if mtp := lockDeviceMTP(device); mtp != entity.Websockets {
		t.Fatalf("expected ws, got %s", mtp)
	}

	device2 := entity.Device{
		SN:    "SN-STOMP",
		Stomp: entity.Online,
	}
	if mtp := lockDeviceMTP(device2); mtp != entity.Stomp {
		t.Fatalf("expected stomp, got %s", mtp)
	}
}

func TestLockDeviceMTP_PriorityOrder(t *testing.T) {
	// All transports online: CWMP should win
	device := entity.Device{
		SN:         "SN-ALL",
		Cwmp:       entity.Online,
		Mqtt:       entity.Online,
		Websockets: entity.Online,
		Stomp:      entity.Online,
	}
	if mtp := lockDeviceMTP(device); mtp != "cwmp" {
		t.Fatalf("expected cwmp as highest priority, got %s", mtp)
	}

	// No CWMP, but MQTT + WS + STOMP: MQTT should win
	device.Cwmp = entity.Offline
	if mtp := lockDeviceMTP(device); mtp != entity.Mqtt {
		t.Fatalf("expected mqtt as second priority, got %s", mtp)
	}

	// No CWMP or MQTT: WS should win
	device.Mqtt = entity.Offline
	if mtp := lockDeviceMTP(device); mtp != entity.Websockets {
		t.Fatalf("expected ws as third priority, got %s", mtp)
	}

	// Only STOMP left
	device.Websockets = entity.Offline
	if mtp := lockDeviceMTP(device); mtp != entity.Stomp {
		t.Fatalf("expected stomp as last priority, got %s", mtp)
	}
}
