package bridge

import (
	"errors"

	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_record"
	"google.golang.org/protobuf/proto"
)

var errUSPRecordMsgIDMissing = errors.New("usp record msg_id missing")

// extractUSPMsgIDFromRecord returns the USP msg_id from a protobuf Record payload.
func extractUSPMsgIDFromRecord(data []byte) (string, error) {
	var record usp_record.Record
	if err := proto.Unmarshal(data, &record); err != nil {
		return "", err
	}

	noSession := record.GetNoSessionContext()
	if noSession == nil || len(noSession.Payload) == 0 {
		return "", errUSPRecordMsgIDMissing
	}

	var msg usp_msg.Msg
	if err := proto.Unmarshal(noSession.Payload, &msg); err != nil {
		return "", err
	}
	if msg.Header == nil || msg.Header.MsgId == "" {
		return "", errUSPRecordMsgIDMissing
	}
	return msg.Header.MsgId, nil
}
