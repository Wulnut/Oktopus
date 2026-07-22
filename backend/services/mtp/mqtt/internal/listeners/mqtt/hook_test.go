package mqtt

import (
	"bytes"
	"sync/atomic"
	"testing"

	mqttserver "github.com/mochi-co/mqtt/v2"
	"github.com/mochi-co/mqtt/v2/packets"
)

func TestShouldPublishOfflineSkipsTakenOverClient(t *testing.T) {
	client := &mqttserver.Client{}
	client.Stop(packets.ErrSessionTakenOver)

	if shouldPublishOffline(client) {
		t.Fatal("taken-over client must not publish offline status")
	}
}

func TestShouldPublishOfflineAllowsNormalDisconnect(t *testing.T) {
	client := &mqttserver.Client{}

	if !shouldPublishOffline(client) {
		t.Fatal("normally disconnected client must publish offline status")
	}
}

func TestOnSubscribedPreservesClientWill(t *testing.T) {
	originalServer := server
	server = mqttserver.New(&mqttserver.Options{})
	t.Cleanup(func() {
		server = originalServer
	})

	client := &mqttserver.Client{
		Properties: mqttserver.ClientProperties{
			Will: mqttserver.Will{
				TopicName: "device/original-will",
				Payload:   []byte("original"),
				Qos:       2,
				Retain:    true,
			},
		},
	}
	packet := packets.Packet{
		Filters: []packets.Subscription{
			{Filter: "oktopus/usp/v1/telkomsel/agent/081074484848"},
		},
	}

	new(MyHook).OnSubscribed(client, packet, nil)

	if client.Properties.Will.TopicName != "device/original-will" {
		t.Fatalf("will topic was overwritten: %q", client.Properties.Will.TopicName)
	}
	if !bytes.Equal(client.Properties.Will.Payload, []byte("original")) {
		t.Fatalf("will payload was overwritten: %q", client.Properties.Will.Payload)
	}
	if client.Properties.Will.Qos != 2 || !client.Properties.Will.Retain {
		t.Fatalf("will options were overwritten: qos=%d retain=%t", client.Properties.Will.Qos, client.Properties.Will.Retain)
	}
}

func TestOnSubscribedDisablesStatusWill(t *testing.T) {
	originalServer := server
	server = mqttserver.New(&mqttserver.Options{})
	t.Cleanup(func() {
		server = originalServer
	})

	client := &mqttserver.Client{
		Properties: mqttserver.ClientProperties{
			Will: mqttserver.Will{
				TopicName: "oktopus/usp/v1/telkomsel/status/081074484848",
				Payload:   []byte("0"),
				Qos:       1,
			},
		},
	}
	atomic.StoreUint32(&client.Properties.Will.Flag, 1)
	packet := packets.Packet{
		Filters: []packets.Subscription{
			{Filter: "oktopus/usp/v1/telkomsel/agent/081074484848"},
		},
	}

	new(MyHook).OnSubscribed(client, packet, nil)

	if atomic.LoadUint32(&client.Properties.Will.Flag) != 0 {
		t.Fatal("platform status will must be disabled so it cannot bypass takeover handling")
	}
}
