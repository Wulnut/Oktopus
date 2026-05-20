package handler

import (
	"context"
	"encoding/json"
	"oktopUSP/backend/services/acs/internal/config"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oleiade/lane"
)

const Version = "1.0.0"

type Request struct {
	Id       string
	User     string
	Password string
	CwmpMsg  []byte
	Time     time.Time
	Callback chan []byte
}

type CPE struct {
	SerialNumber         string
	TenantSlug           string
	Manufacturer         string
	OUI                  string
	ConnectionRequestURL string
	SoftwareVersion      string
	ExternalIPAddress    string
	Queue                *lane.Queue
	Waiting              *Request
	HardwareVersion      string
	LastConnection       time.Time
	DataModel            string
	Username             string
	Password             string
}

type Message struct {
	SerialNumber string
	Message      string
}

type WsMessage struct {
	Cmd string
}

type NatsSendMessage struct {
	MsgType string
	Data    json.RawMessage
}

type MsgCPEs struct {
	CPES map[string]CPE
}

type Handler struct {
	pub       func(string, []byte) error
	sub       func(string, func(*nats.Msg)) error
	Cpes      map[string]CPE
	mu        sync.RWMutex
	acsConfig config.Acs

	// js is the JetStream client used to access per-tenant KV buckets
	// (cwmp-conn-rq-<tenant>) that hold Connection Request credentials
	// provisioned to each CPE. Nil in tests that do not exercise KV paths.
	js jetstream.JetStream
	// kvCache maps tenant slug -> resolved KeyValue handle. Guarded by kvMu.
	// We lazily resolve on first access to avoid coupling ACS startup to
	// tenant creation order.
	kvCache map[string]jetstream.KeyValue
	kvMu    sync.Mutex
	bgCtx   context.Context
}

const (
	NATS_CWMP_SUBJECT_PREFIX         = "cwmp.v1."
	NATS_CWMP_ADAPTER_SUBJECT_PREFIX = "cwmp-adapter.v1."
	NATS_ADAPTER_SUBJECT_PREFIX      = "adapter.v1."
	DEFAULT_TENANT                   = "default"
)

func NewHandler(
	pub func(string, []byte) error,
	sub func(string, func(*nats.Msg)) error,
	cAcs config.Acs,
) *Handler {
	return NewHandlerWithJS(pub, sub, cAcs, nil)
}

// NewHandlerWithJS is like NewHandler but plumbs in a JetStream client so the
// CWMP session handler can read and write Connection Request credentials in
// per-tenant KV buckets. Pass nil to disable provisioning (tests).
func NewHandlerWithJS(
	pub func(string, []byte) error,
	sub func(string, func(*nats.Msg)) error,
	cAcs config.Acs,
	js jetstream.JetStream,
) *Handler {
	return &Handler{
		pub:       pub,
		sub:       sub,
		Cpes:      make(map[string]CPE),
		acsConfig: cAcs,
		js:        js,
		kvCache:   make(map[string]jetstream.KeyValue),
		bgCtx:     context.Background(),
	}
}
