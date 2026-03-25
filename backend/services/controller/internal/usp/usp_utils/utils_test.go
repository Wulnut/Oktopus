package usp_utils

import (
	"testing"

	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_record"
	"google.golang.org/protobuf/proto"
)

// --- GET message ---

func TestNewGetMsg_SetsCorrectMsgType(t *testing.T) {
	msg := NewGetMsg(usp_msg.Get{
		ParamPaths: []string{"Device.DeviceInfo."},
		MaxDepth:   2,
	})
	if msg.Header.MsgType != usp_msg.Header_GET {
		t.Errorf("Expected MsgType GET, got %v", msg.Header.MsgType)
	}
}

func TestNewGetMsg_HasValidMsgId(t *testing.T) {
	msg := NewGetMsg(usp_msg.Get{ParamPaths: []string{"Device."}})
	if msg.Header.MsgId == "" {
		t.Error("MsgId should not be empty")
	}
	// UUID format: 8-4-4-4-12
	if len(msg.Header.MsgId) != 36 {
		t.Errorf("MsgId should be UUID format (36 chars), got %d: %s", len(msg.Header.MsgId), msg.Header.MsgId)
	}
}

func TestNewGetMsg_PreservesParamPaths(t *testing.T) {
	paths := []string{"Device.DeviceInfo.", "Device.WiFi."}
	msg := NewGetMsg(usp_msg.Get{ParamPaths: paths, MaxDepth: 3})
	get := msg.Body.GetRequest().GetGet()
	if get == nil {
		t.Fatal("Get body is nil")
	}
	if len(get.ParamPaths) != 2 {
		t.Fatalf("Expected 2 param paths, got %d", len(get.ParamPaths))
	}
	if get.ParamPaths[0] != "Device.DeviceInfo." {
		t.Errorf("Expected first path Device.DeviceInfo., got %s", get.ParamPaths[0])
	}
	if get.MaxDepth != 3 {
		t.Errorf("Expected MaxDepth 3, got %d", get.MaxDepth)
	}
}

func TestNewGetMsg_SerializesToValidProtobuf(t *testing.T) {
	msg := NewGetMsg(usp_msg.Get{ParamPaths: []string{"Device."}})
	data, err := proto.Marshal(&msg)
	if err != nil {
		t.Fatalf("Failed to marshal GET message: %v", err)
	}
	var decoded usp_msg.Msg
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal GET message: %v", err)
	}
	if decoded.Header.MsgType != usp_msg.Header_GET {
		t.Error("Decoded message has wrong type")
	}
}

// --- SET message ---

func TestNewSetMsg_SetsCorrectMsgType(t *testing.T) {
	msg := NewSetMsg(usp_msg.Set{
		AllowPartial: false,
		UpdateObjs: []*usp_msg.Set_UpdateObject{
			{
				ObjPath: "Device.WiFi.SSID.1.",
				ParamSettings: []*usp_msg.Set_UpdateParamSetting{
					{Param: "SSID", Value: "TestNetwork"},
				},
			},
		},
	})
	if msg.Header.MsgType != usp_msg.Header_SET {
		t.Errorf("Expected MsgType SET, got %v", msg.Header.MsgType)
	}
}

func TestNewSetMsg_PreservesParams(t *testing.T) {
	msg := NewSetMsg(usp_msg.Set{
		UpdateObjs: []*usp_msg.Set_UpdateObject{
			{
				ObjPath: "Device.WiFi.SSID.1.",
				ParamSettings: []*usp_msg.Set_UpdateParamSetting{
					{Param: "SSID", Value: "MyNet"},
				},
			},
		},
	})
	set := msg.Body.GetRequest().GetSet()
	if set == nil {
		t.Fatal("Set body is nil")
	}
	if len(set.UpdateObjs) != 1 {
		t.Fatalf("Expected 1 update object, got %d", len(set.UpdateObjs))
	}
	if set.UpdateObjs[0].ObjPath != "Device.WiFi.SSID.1." {
		t.Errorf("Wrong ObjPath: %s", set.UpdateObjs[0].ObjPath)
	}
	if set.UpdateObjs[0].ParamSettings[0].Value != "MyNet" {
		t.Errorf("Wrong param value: %s", set.UpdateObjs[0].ParamSettings[0].Value)
	}
}

func TestNewSetMsg_SerializesToValidProtobuf(t *testing.T) {
	msg := NewSetMsg(usp_msg.Set{
		UpdateObjs: []*usp_msg.Set_UpdateObject{
			{ObjPath: "Device.Test.", ParamSettings: []*usp_msg.Set_UpdateParamSetting{{Param: "Key", Value: "Val"}}},
		},
	})
	data, err := proto.Marshal(&msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	if len(data) == 0 {
		t.Error("Marshaled data is empty")
	}
}

// --- ADD message ---

func TestNewCreateMsg_SetsCorrectMsgType(t *testing.T) {
	msg := NewCreateMsg(usp_msg.Add{
		AllowPartial: false,
		CreateObjs: []*usp_msg.Add_CreateObject{
			{ObjPath: "Device.Bridge."},
		},
	})
	if msg.Header.MsgType != usp_msg.Header_ADD {
		t.Errorf("Expected MsgType ADD, got %v", msg.Header.MsgType)
	}
}

func TestNewCreateMsg_PreservesObjPath(t *testing.T) {
	msg := NewCreateMsg(usp_msg.Add{
		CreateObjs: []*usp_msg.Add_CreateObject{
			{ObjPath: "Device.Bridge."},
		},
	})
	add := msg.Body.GetRequest().GetAdd()
	if add == nil {
		t.Fatal("Add body is nil")
	}
	if add.CreateObjs[0].ObjPath != "Device.Bridge." {
		t.Errorf("Wrong ObjPath: %s", add.CreateObjs[0].ObjPath)
	}
}

// --- DELETE message ---

func TestNewDelMsg_SetsCorrectMsgType(t *testing.T) {
	msg := NewDelMsg(usp_msg.Delete{
		ObjPaths: []string{"Device.Bridge.1."},
	})
	if msg.Header.MsgType != usp_msg.Header_DELETE {
		t.Errorf("Expected MsgType DELETE, got %v", msg.Header.MsgType)
	}
}

func TestNewDelMsg_PreservesObjPaths(t *testing.T) {
	msg := NewDelMsg(usp_msg.Delete{
		ObjPaths: []string{"Device.Bridge.1.", "Device.Bridge.2."},
	})
	del := msg.Body.GetRequest().GetDelete()
	if del == nil {
		t.Fatal("Delete body is nil")
	}
	if len(del.ObjPaths) != 2 {
		t.Fatalf("Expected 2 paths, got %d", len(del.ObjPaths))
	}
}

// --- OPERATE message ---

func TestNewOperateMsg_SetsCorrectMsgType(t *testing.T) {
	msg := NewOperateMsg(usp_msg.Operate{
		Command:    "Device.Reboot()",
		CommandKey: "reboot-1",
		SendResp:   true,
	})
	if msg.Header.MsgType != usp_msg.Header_OPERATE {
		t.Errorf("Expected MsgType OPERATE, got %v", msg.Header.MsgType)
	}
}

func TestNewOperateMsg_PreservesCommandAndArgs(t *testing.T) {
	msg := NewOperateMsg(usp_msg.Operate{
		Command:    "Device.DeviceInfo.FirmwareImage.1.Download()",
		CommandKey: "fw-download",
		SendResp:   true,
		InputArgs:  map[string]string{"URL": "http://example.com/fw.bin", "AutoActivate": "true"},
	})
	op := msg.Body.GetRequest().GetOperate()
	if op == nil {
		t.Fatal("Operate body is nil")
	}
	if op.Command != "Device.DeviceInfo.FirmwareImage.1.Download()" {
		t.Errorf("Wrong command: %s", op.Command)
	}
	if op.InputArgs["URL"] != "http://example.com/fw.bin" {
		t.Errorf("Wrong URL input arg: %s", op.InputArgs["URL"])
	}
	if op.InputArgs["AutoActivate"] != "true" {
		t.Errorf("Wrong AutoActivate: %s", op.InputArgs["AutoActivate"])
	}
	if !op.SendResp {
		t.Error("SendResp should be true")
	}
}

// --- GET_SUPPORTED_DM ---

func TestNewGetSupportedParametersMsg_SetsCorrectMsgType(t *testing.T) {
	msg := NewGetSupportedParametersMsg(usp_msg.GetSupportedDM{
		ObjPaths:       []string{"Device."},
		FirstLevelOnly: true,
		ReturnParams:   true,
	})
	if msg.Header.MsgType != usp_msg.Header_GET_SUPPORTED_DM {
		t.Errorf("Expected GET_SUPPORTED_DM, got %v", msg.Header.MsgType)
	}
}

// --- GET_INSTANCES ---

func TestNewGetParametersInstancesMsg_SetsCorrectMsgType(t *testing.T) {
	msg := NewGetParametersInstancesMsg(usp_msg.GetInstances{
		ObjPaths:       []string{"Device.Bridge."},
		FirstLevelOnly: true,
	})
	if msg.Header.MsgType != usp_msg.Header_GET_INSTANCES {
		t.Errorf("Expected GET_INSTANCES, got %v", msg.Header.MsgType)
	}
}

// --- NOTIFY ---

func TestNewNotifyMsg_SetsCorrectMsgType(t *testing.T) {
	msg := NewNotifyMsg(usp_msg.Notify{})
	if msg.Header.MsgType != usp_msg.Header_NOTIFY {
		t.Errorf("Expected NOTIFY, got %v", msg.Header.MsgType)
	}
}

// --- USP Record ---

func TestNewUspRecord_SetsVersionAndEndpoints(t *testing.T) {
	payload := []byte("test-payload")
	record := NewUspRecord(payload, "device-SN123")

	if record.Version != "1.0" {
		t.Errorf("Expected Version 1.0, got %s", record.Version)
	}
	if record.FromId != "oktopusController" {
		t.Errorf("Expected FromId oktopusController, got %s", record.FromId)
	}
	if record.ToId != "device-SN123" {
		t.Errorf("Expected ToId device-SN123, got %s", record.ToId)
	}
	if record.PayloadSecurity != usp_record.Record_PLAINTEXT {
		t.Errorf("Expected PLAINTEXT security, got %v", record.PayloadSecurity)
	}
}

func TestNewUspRecord_ContainsPayload(t *testing.T) {
	payload := []byte("test-payload-data")
	record := NewUspRecord(payload, "SN1")
	nsc := record.GetNoSessionContext()
	if nsc == nil {
		t.Fatal("NoSessionContext is nil")
	}
	if string(nsc.Payload) != "test-payload-data" {
		t.Errorf("Payload mismatch: %s", string(nsc.Payload))
	}
}

func TestNewUspRecord_SerializesToValidProtobuf(t *testing.T) {
	payload := []byte("data")
	record := NewUspRecord(payload, "SN1")
	data, err := proto.Marshal(&record)
	if err != nil {
		t.Fatalf("Failed to marshal record: %v", err)
	}
	var decoded usp_record.Record
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal record: %v", err)
	}
	if decoded.ToId != "SN1" {
		t.Errorf("Decoded ToId mismatch: %s", decoded.ToId)
	}
}

// --- Unique MsgId per call ---

func TestAllMsgConstructors_ProduceUniqueMsgIds(t *testing.T) {
	ids := make(map[string]bool)
	msgs := []usp_msg.Msg{
		NewGetMsg(usp_msg.Get{ParamPaths: []string{"Device."}}),
		NewGetMsg(usp_msg.Get{ParamPaths: []string{"Device."}}),
		NewSetMsg(usp_msg.Set{}),
		NewCreateMsg(usp_msg.Add{}),
		NewDelMsg(usp_msg.Delete{}),
		NewOperateMsg(usp_msg.Operate{Command: "Device.Reboot()"}),
	}
	for i, msg := range msgs {
		if ids[msg.Header.MsgId] {
			t.Errorf("Duplicate MsgId at index %d: %s", i, msg.Header.MsgId)
		}
		ids[msg.Header.MsgId] = true
	}
}
