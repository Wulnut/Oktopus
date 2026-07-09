package bridge

import (
	"testing"

	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_utils"
	"google.golang.org/protobuf/proto"
)

func TestExtractUSPMsgIDFromRecord_Request(t *testing.T) {
	msg := usp_utils.NewGetMsg(usp_msg.Get{ParamPaths: []string{"Device.DeviceInfo."}})
	payload, err := proto.Marshal(&msg)
	if err != nil {
		t.Fatal(err)
	}

	record := usp_utils.NewUspRecord(payload, "SN1")
	body, err := proto.Marshal(&record)
	if err != nil {
		t.Fatal(err)
	}

	got, err := extractUSPMsgIDFromRecord(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != msg.Header.MsgId {
		t.Fatalf("expected msg_id %q, got %q", msg.Header.MsgId, got)
	}
}

func TestExtractUSPMsgIDFromRecord_InvalidData(t *testing.T) {
	if _, err := extractUSPMsgIDFromRecord([]byte("not-protobuf")); err == nil {
		t.Fatal("expected error for invalid protobuf")
	}
}
