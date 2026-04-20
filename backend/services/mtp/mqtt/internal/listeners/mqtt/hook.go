package mqtt

import (
	"broker/internal/config"
	"broker/internal/nats"
	"bytes"
	"context"
	"log"
	"strings"

	"github.com/mochi-co/mqtt/v2"
	"github.com/mochi-co/mqtt/v2/hooks/auth"
	"github.com/mochi-co/mqtt/v2/packets"
	"github.com/nats-io/nats.go/jetstream"
)

// MyHook handles device lifecycle events (subscribe, disconnect, packet encode).
type MyHook struct {
	mqtt.HookBase
}

// NatsAuthHook handles MQTT authentication and ACL using per-tenant NATS KV buckets.
// Username format: <tenant_slug>/<endpoint_id>
// Credentials are looked up in bucket "devices-auth-<tenant_slug>".
type NatsAuthHook struct {
	mqtt.HookBase
	js jetstream.JetStream
}

func (h *MyHook) ID() string {
	return "events-controller"
}

func (h *MyHook) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnSubscribed,
		mqtt.OnDisconnect,
		mqtt.OnClientExpired,
	}, []byte{b})
}

func (h *MyHook) Init(config any) error {
	h.Log.Info().Msg("initialised")
	return nil
}

func (h *MyHook) OnClientExpired(cl *mqtt.Client) {
	log.Printf("Client id %s expired", cl.ID)
}

// OnDisconnect publishes offline status to the tenant-scoped status topic.
func (h *MyHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	tenant, device := getTenantAndDevice(cl)
	if device == "" {
		return
	}

	statusTopic := "oktopus/usp/v1/" + tenant + "/status/" + device
	if pubErr := server.Publish(statusTopic, []byte("0"), false, 1); pubErr != nil {
		log.Println("server publish error:", pubErr)
	}
}

// OnSubscribed detects device subscription, stores tenant context, and publishes online status.
// Expects subscribe topic: oktopus/usp/v1/<tenant>/agent/<device>
func (h *MyHook) OnSubscribed(cl *mqtt.Client, pk packets.Packet, reasonCodes []byte) {
	filter := pk.Filters[0].Filter
	// Only process device agent subscriptions
	if !strings.Contains(filter, "/agent/") {
		return
	}

	tenant, device := parseTenantDeviceFromTopic(filter)
	if device == "" {
		return
	}

	// Store tenant in client user properties for OnDisconnect
	storeTenantInClient(cl, tenant, device)

	// Set will message for ungraceful disconnect
	statusTopic := "oktopus/usp/v1/" + tenant + "/status/" + device
	cl.Properties.Will = mqtt.Will{
		Qos:       1,
		TopicName: statusTopic,
		Payload:   []byte("0"),
		Retain:    false,
	}

	log.Printf("new device: tenant=%s device=%s", tenant, device)
	if err := server.Publish(statusTopic, []byte("1"), false, 1); err != nil {
		log.Println("server publish error:", err)
	}
}

func (h *NatsAuthHook) ID() string {
	return "device-auth"
}

func (h *NatsAuthHook) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnConnectAuthenticate,
		mqtt.OnACLCheck,
	}, []byte{b})
}

func (h *NatsAuthHook) Init(c any) error {
	js, _ := nats.StartNatsClient(c.(config.Nats))
	h.js = js
	h.Log.Info().Msg("initialised device auth nats hook")
	return nil
}

// OnConnectAuthenticate validates device credentials from per-tenant KV bucket.
// Username format: <tenant_slug>/<endpoint_id>
// Looks up in bucket "devices-auth-<tenant_slug>" with key = username.
func (h *NatsAuthHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	username := string(pk.Connect.Username)

	tenant, _ := parseTenantDevice(username)
	if tenant == "" {
		log.Printf("auth: invalid username format (expected tenant/device): %s", username)
		return false
	}

	bucketName := "devices-auth-" + tenant
	kv, err := h.js.KeyValue(context.TODO(), bucketName)
	if err != nil {
		log.Printf("auth: tenant KV bucket %s not found: %v", bucketName, err)
		return false
	}

	entry, err := kv.Get(context.TODO(), username)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			log.Printf("auth: credential not found for user %s in bucket %s", username, bucketName)
		} else {
			log.Printf("auth: error getting credential for %s: %v", username, err)
		}
		return false
	}

	if bytes.Equal(entry.Value(), pk.Connect.Password) {
		return true
	}

	log.Printf("auth: password mismatch for user %s", username)
	return false
}

// OnACLCheck validates topic access for authenticated devices.
// Controller user ("oktopusController") gets full access.
// Device topics must match the tenant-prefixed pattern.
func (h *NatsAuthHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	username := string(cl.Properties.Username)

	// Controller has full access
	if username == "oktopusController" {
		return true
	}

	tenant, device := parseTenantDevice(username)
	if tenant == "" || device == "" {
		return false
	}

	if !write {
		// Devices can read (subscribe to) their agent topic
		allowedRead := []string{
			"oktopus/usp/v1/" + tenant + "/agent/" + device,
		}
		for _, allowed := range allowedRead {
			if _, ok := auth.MatchTopic(allowed, topic); ok {
				return true
			}
		}
		return false
	}

	// Devices can write (publish) to controller, api, async, and status topics
	allowedWrite := []string{
		"oktopus/usp/v1/" + tenant + "/controller/" + device,
		"oktopus/usp/v1/" + tenant + "/api/" + device,
		"oktopus/usp/v1/" + tenant + "/async/" + device,
		"oktopus/usp/v1/" + tenant + "/status/" + device,
	}
	for _, allowed := range allowedWrite {
		if _, ok := auth.MatchTopic(allowed, topic); ok {
			return true
		}
	}

	return false
}

// parseTenantDevice splits "tenant/device" username into parts.
func parseTenantDevice(username string) (tenant, device string) {
	parts := strings.SplitN(username, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}

// parseTenantDeviceFromTopic extracts tenant and device from MQTT topic.
// Format: oktopus/usp/v1/<tenant>/agent/<device>
func parseTenantDeviceFromTopic(topic string) (tenant, device string) {
	parts := strings.Split(topic, "/")
	if len(parts) >= 6 && parts[4] == "agent" {
		return parts[3], parts[5]
	}
	// Fallback for old format: oktopus/usp/v1/agent/<device>
	if len(parts) >= 5 && parts[3] == "agent" {
		return "default", parts[4]
	}
	return "", ""
}

// getTenantAndDevice reads tenant and device from client user properties.
func getTenantAndDevice(cl *mqtt.Client) (tenant, device string) {
	for _, prop := range cl.Properties.Props.User {
		switch prop.Key {
		case "tenant":
			tenant = prop.Val
		case "device":
			device = prop.Val
		}
	}
	return
}

// storeTenantInClient saves tenant and device in client user properties
// so they're available in OnDisconnect.
func storeTenantInClient(cl *mqtt.Client, tenant, device string) {
	cl.Properties.Props.User = []packets.UserProperty{
		{Key: "tenant", Val: tenant},
		{Key: "device", Val: device},
	}
}
