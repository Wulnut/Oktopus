package bridge

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_utils"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// TestNatsUspInteraction_ConcurrentSameDevice verifies that two concurrent
// requests to the same device serial number do not cross-contaminate responses.
// Each request is correlated by USP msg_id on the shared NATS subject.
func TestNatsUspInteraction_ConcurrentSameDevice(t *testing.T) {
	// This test requires a live NATS connection.
	// Use NATS_URL env var or skip.
	natsURL := natsTestURL()
	if natsURL == "" {
		t.Skip("NATS_URL not set -- skipping bridge integration test")
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	sn := "TEST-DEVICE-001"
	subSubj := "device.usp.v1." + sn + ".api"
	pubSubj := "test-adapter.usp.v1." + sn + ".api"

	// Mock responder: for each message received on pubSubj, reply on subSubj
	// with the message data prefixed by a unique marker.
	responderSub, err := nc.Subscribe(pubSubj, func(msg *nats.Msg) {
		// Echo back with a marker so we can tell which response belongs to which request
		nc.Publish(subSubj, msg.Data)
	})
	if err != nil {
		t.Fatalf("Failed to subscribe responder: %v", err)
	}
	defer responderSub.Unsubscribe()

	var wg sync.WaitGroup
	results := make([][]byte, 2)
	errors := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			payload := uspRecordForTest(t, "request-"+string(rune('A'+idx)))
			results[idx], errors[idx] = NatsUspInteraction(subSubj, pubSubj, payload, w, nc)
		}(i)
	}

	wg.Wait()

	// Both should succeed
	for i := 0; i < 2; i++ {
		if errors[i] != nil {
			t.Errorf("Request %d failed: %v", i, errors[i])
		}
	}

	// Key assertion: each request should get its own response, not the other's.
	// With the current bug (shared subject), one request may get the other's response.
	if len(results[0]) > 0 && len(results[1]) > 0 {
		if string(results[0]) == string(results[1]) {
			t.Error("Both concurrent requests got the same response -- subject collision detected")
		}
	}
}

// TestNatsCustomReq_DoesNotTimeout verifies that NatsCustomReq publishes before
// waiting for the response.
func TestNatsCustomReq_DoesNotTimeout(t *testing.T) {
	natsURL := natsTestURL()
	if natsURL == "" {
		t.Skip("NATS_URL not set -- skipping bridge integration test")
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	subSubj := "test.custom.sub"
	pubSubj := "test.custom.pub"

	// Responder: when we get a message on pubSubj, reply on subSubj
	responderSub, err := nc.Subscribe(pubSubj, func(msg *nats.Msg) {
		nc.Publish(subSubj, []byte(`"ok"`))
	})
	if err != nil {
		t.Fatalf("Failed to subscribe responder: %v", err)
	}
	defer responderSub.Unsubscribe()

	w := httptest.NewRecorder()

	start := time.Now()
	result, err := NatsCustomReq[*string](subSubj, pubSubj, []byte("test"), w, nc)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("NatsCustomReq failed: %v", err)
	}
	if result == nil {
		t.Fatal("NatsCustomReq returned nil result")
	}
	got, ok := result.(*string)
	if !ok || got == nil || *got != "ok" {
		t.Fatalf("NatsCustomReq result = %v, want %q", result, "ok")
	}

	// If it took close to NATS_REQUEST_TIMEOUT (10s), it timed out instead of
	// receiving the response.
	if elapsed > 5*time.Second {
		t.Errorf("NatsCustomReq took %v -- likely timed out instead of receiving response (bug: subscribe before publish)", elapsed)
	}
}

// TestNatsUspInteraction_SingleRequest_Success verifies the happy path.
func TestNatsCustomReq_PublishFailureDoesNotWriteAfterReturn(t *testing.T) {
	natsURL := natsTestURL()
	if natsURL == "" {
		t.Skip("NATS_URL not set -- skipping bridge integration test")
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	nc.Close()

	w := &returnAwareWriter{ResponseRecorder: httptest.NewRecorder()}
	_, err = natsCustomReq[*string](
		"test.custom.publish-failure.sub",
		"test.custom.publish-failure.pub",
		[]byte("request"),
		w,
		nc,
		25*time.Millisecond,
	)
	if err == nil {
		t.Fatal("expected publish failure")
	}
	w.returned.Store(true)
	time.Sleep(75 * time.Millisecond)
	if got := w.writesAfterReturn.Load(); got != 0 {
		t.Fatalf("response writer was used %d times after NatsCustomReq returned", got)
	}
}

func TestNatsUspInteraction_SingleRequest_Success(t *testing.T) {
	natsURL := natsTestURL()
	if natsURL == "" {
		t.Skip("NATS_URL not set -- skipping bridge integration test")
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	subSubj := "test.usp.single.sub"
	pubSubj := "test.usp.single.pub"
	expected := uspRecordForTest(t, "single-request")

	responderSub, err := nc.Subscribe(pubSubj, func(msg *nats.Msg) {
		nc.Publish(subSubj, expected)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer responderSub.Unsubscribe()

	w := httptest.NewRecorder()
	result, err := NatsUspInteraction(subSubj, pubSubj, expected, w, nc)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if string(result) != string(expected) {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// TestNatsUspInteraction_Timeout verifies timeout behavior.
func TestNatsUspInteraction_Timeout(t *testing.T) {
	natsURL := natsTestURL()
	if natsURL == "" {
		t.Skip("NATS_URL not set -- skipping bridge integration test")
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// No responder -- should timeout
	w := httptest.NewRecorder()
	_, err = natsUspInteraction("test.timeout.sub", "test.timeout.pub", uspRecordForTest(t, "timeout-request"), w, nc, 50*time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	if err != errNatsRequestTimeout {
		t.Errorf("Expected errNatsRequestTimeout, got %v", err)
	}
}

func TestNatsUspInteraction_InvalidRecordFailsBeforeNATS(t *testing.T) {
	w := httptest.NewRecorder()

	_, err := natsUspInteraction("unused.sub", "unused.pub", []byte("not-protobuf"), w, nil, time.Second)
	if !errors.Is(err, errInvalidUSPRequest) {
		t.Fatalf("expected errInvalidUSPRequest, got %v", err)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestNatsUspInteraction_PublishFailureDoesNotWriteAfterReturn(t *testing.T) {
	natsURL := natsTestURL()
	if natsURL == "" {
		t.Skip("NATS_URL not set -- skipping bridge integration test")
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	nc.Close()

	w := &returnAwareWriter{ResponseRecorder: httptest.NewRecorder()}
	_, err = natsUspInteraction(
		"test.publish-failure.sub",
		"test.publish-failure.pub",
		uspRecordForTest(t, "publish-failure"),
		w,
		nc,
		25*time.Millisecond,
	)
	if err == nil {
		t.Fatal("expected publish failure")
	}
	w.returned.Store(true)
	time.Sleep(75 * time.Millisecond)
	if got := w.writesAfterReturn.Load(); got != 0 {
		t.Fatalf("response writer was used %d times after NatsUspInteraction returned", got)
	}
}

type returnAwareWriter struct {
	*httptest.ResponseRecorder
	returned          atomic.Bool
	writesAfterReturn atomic.Int32
}

func (w *returnAwareWriter) WriteHeader(statusCode int) {
	if w.returned.Load() {
		w.writesAfterReturn.Add(1)
	}
	w.ResponseRecorder.WriteHeader(statusCode)
}

func (w *returnAwareWriter) Write(data []byte) (int, error) {
	if w.returned.Load() {
		w.writesAfterReturn.Add(1)
	}
	return w.ResponseRecorder.Write(data)
}

func uspRecordForTest(t *testing.T, msgID string) []byte {
	t.Helper()

	msg := usp_utils.NewGetMsg(usp_msg.Get{ParamPaths: []string{"Device.DeviceInfo."}})
	msg.Header.MsgId = msgID
	payload, err := proto.Marshal(&msg)
	if err != nil {
		t.Fatal(err)
	}
	record := usp_utils.NewUspRecord(payload, "TEST-DEVICE-001")
	body, err := proto.Marshal(&record)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func natsTestURL() string {
	url := os.Getenv("NATS_TEST_URL")
	return url
}
