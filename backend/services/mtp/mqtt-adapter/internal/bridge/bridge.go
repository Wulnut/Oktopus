package bridge

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/OktopUSP/oktopus/backend/services/mqtt-adapter/internal/config"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sys/unix"
)

const (
	OFFLINE = iota
	ONLINE
)

type msgAnswer struct {
	Code int
	Msg  any
}

const NATS_MQTT_SUBJECT_PREFIX = "mqtt.usp.v1."
const NATS_MQTT_ADAPTER_SUBJECT_PREFIX = "mqtt-adapter.usp.v1."
const DEVICE_SUBJECT_PREFIX = "device.usp.v1."
const MQTT_TOPIC_PREFIX = "oktopus/usp/v1/"
const DEFAULT_TENANT = "default"

type (
	Publisher  func(string, []byte) error
	Subscriber func(string, func(*nats.Msg)) error
)

type Bridge struct {
	Pub   Publisher
	Sub   Subscriber
	Mqtt  config.Mqtt
	kv    jetstream.KeyValue
	Ctx   context.Context
	Debug bool
}

func NewBridge(p Publisher, s Subscriber, ctx context.Context, m config.Mqtt, kv jetstream.KeyValue, debug bool) *Bridge {
	return &Bridge{
		Pub:   p,
		Sub:   s,
		Mqtt:  m,
		Ctx:   ctx,
		kv:    kv,
		Debug: debug,
	}
}

func (b *Bridge) debugf(format string, args ...any) {
	if b.Debug {
		log.Printf(format, args...)
	}
}

func (b *Bridge) StartBridge(serverUrl, clientId string) {

	broker, _ := url.Parse(serverUrl)

	status := make(chan *paho.Publish)
	controller := make(chan *paho.Publish)
	apiMsg := make(chan *paho.Publish)
	asyncMsg := make(chan *paho.Publish)

	go b.mqttMessageHandler(status, controller, apiMsg, asyncMsg)

	pahoClientConfig := buildClientConfig(status, controller, apiMsg, asyncMsg, clientId)

	autopahoClientConfig := autopaho.ClientConfig{
		BrokerUrls: []*url.URL{
			broker,
		},
		KeepAlive:         30,
		ConnectRetryDelay: 5 * time.Second,
		ConnectTimeout:    5 * time.Second,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
			log.Printf("Connected to MQTT broker--> %s", serverUrl)
			subscribe(b.Mqtt.Ctx, b.Mqtt.Qos, cm)
		},
		OnConnectError: func(err error) {
			log.Printf("Error while attempting connection: %s\n", err)
		},
		ClientConfig: *pahoClientConfig,
		TlsCfg: &tls.Config{
			InsecureSkipVerify: b.Mqtt.SkipVerify,
		},
	}

	b.setMqttPassword()
	if b.Mqtt.Username != "" {
		autopahoClientConfig.SetUsernamePassword(b.Mqtt.Username, []byte(b.Mqtt.Password))
	}

	log.Println("MQTT client id:", pahoClientConfig.ClientID)
	log.Println("MQTT username:", b.Mqtt.Username)
	log.Println("MQTT password: [REDACTED]")

	cm, err := autopaho.NewConnection(b.Ctx, autopahoClientConfig)
	if err != nil {
		log.Fatalln(err)
	}

	b.natsMessageHandler(cm)
}

func (b *Bridge) natsMessageHandler(cm *autopaho.ConnectionManager) {
	b.Sub(NATS_MQTT_ADAPTER_SUBJECT_PREFIX+"*.*.info", func(m *nats.Msg) {

		device := getDeviceFromSubject(m.Subject)
		tenant := extractTenantFromSubject(m.Subject)
		agentTopic := MQTT_TOPIC_PREFIX + tenant + "/agent/" + device
		responseTopic := MQTT_TOPIC_PREFIX + tenant + "/controller/" + device
		b.debugf("mqtt-out info: device=%s tenant=%s agentTopic=%s responseTopic=%s size=%d",
			device, tenant, agentTopic, responseTopic, len(m.Data))
		cm.Publish(b.Ctx, &paho.Publish{
			QoS:     byte(b.Mqtt.Qos),
			Topic:   agentTopic,
			Payload: m.Data,
			Properties: &paho.PublishProperties{
				ResponseTopic: responseTopic,
			},
		})

	})

	b.Sub(NATS_MQTT_ADAPTER_SUBJECT_PREFIX+"*.*.api", func(m *nats.Msg) {

		device := getDeviceFromSubject(m.Subject)
		tenant := extractTenantFromSubject(m.Subject)
		responseTopic := MQTT_TOPIC_PREFIX + tenant + "/api/" + device
		agentTopic := MQTT_TOPIC_PREFIX + tenant + "/agent/" + device
		b.debugf("mqtt-out api: device=%s tenant=%s agentTopic=%s responseTopic=%s size=%d",
			device, tenant, agentTopic, responseTopic, len(m.Data))
		cm.Publish(b.Ctx, &paho.Publish{
			QoS:     byte(b.Mqtt.Qos),
			Topic:   agentTopic,
			Payload: m.Data,
			Properties: &paho.PublishProperties{
				ResponseTopic: responseTopic,
			},
		})

	})

	b.Sub(NATS_MQTT_ADAPTER_SUBJECT_PREFIX+"*.rtt", func(msg *nats.Msg) {

		log.Printf("Received message on rtt subject")
		url := strings.Split(b.Mqtt.Url, "://")[1]
		conn, err := net.Dial("tcp", url)
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
			return
		}
		defer conn.Close()

		info, err := tcpInfo(conn.(*net.TCPConn))
		if err != nil {
			respondMsg(msg.Respond, 500, err.Error())
			return
		}
		rtt := time.Duration(info.Rtt) * time.Microsecond

		respondMsg(msg.Respond, 200, rtt/1000)
	})
}

func getDeviceFromSubject(subject string) string {
	paths := strings.Split(subject, ".")
	device := paths[len(paths)-2]
	return device
}

func extractTenantFromSubject(subject string) string {
	paths := strings.Split(subject, ".")
	if len(paths) >= 5 {
		return paths[3]
	}
	return DEFAULT_TENANT
}

func (b *Bridge) mqttMessageHandler(status, controller, apiMsg, asyncMsg chan *paho.Publish) {
	for {
		select {
		case d := <-status:
			device := getDeviceFromTopic(d.Topic)
			tenant := getTenantFromTopic(d.Topic)
			b.debugf("mqtt-in status: topic=%s device=%s tenant=%s size=%d", d.Topic, device, tenant, len(d.Payload))
			b.Pub(NATS_MQTT_SUBJECT_PREFIX+tenant+"."+device+".status", d.Payload)
		case c := <-controller:
			device := getDeviceFromTopic(c.Topic)
			tenant := getTenantFromTopic(c.Topic)
			natsSubj := NATS_MQTT_SUBJECT_PREFIX + tenant + "." + device + ".info"
			b.debugf("mqtt-in controller: topic=%s device=%s tenant=%s nats=%s size=%d",
				c.Topic, device, tenant, natsSubj, len(c.Payload))
			b.Pub(natsSubj, c.Payload)
		case a := <-apiMsg:
			device := getDeviceFromTopic(a.Topic)
			tenant := getTenantFromTopic(a.Topic)
			natsSubj := DEVICE_SUBJECT_PREFIX + tenant + "." + device + ".api"
			b.debugf("mqtt-in api: topic=%s device=%s tenant=%s nats=%s size=%d",
				a.Topic, device, tenant, natsSubj, len(a.Payload))
			b.Pub(natsSubj, a.Payload)
		case async := <-asyncMsg:
			device := getDeviceFromTopic(async.Topic)
			tenant := getTenantFromTopic(async.Topic)
			b.debugf("mqtt-in async: topic=%s device=%s tenant=%s size=%d", async.Topic, device, tenant, len(async.Payload))
			b.Pub(NATS_MQTT_SUBJECT_PREFIX+tenant+"."+device+".async", async.Payload)
		}
	}
}

func getDeviceFromTopic(topic string) string {
	paths := strings.Split(topic, "/")
	device := paths[len(paths)-1]
	return device
}

// getTenantFromTopic extracts tenant slug from MQTT topic.
// Topic format: oktopus/usp/v1/<tenant>/controller/<device>
func getTenantFromTopic(topic string) string {
	paths := strings.Split(topic, "/")
	if len(paths) >= 4 {
		return paths[3]
	}
	return DEFAULT_TENANT
}

func subscribe(ctx context.Context, qos int, c *autopaho.ConnectionManager) {
	if _, err := c.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{
			{
				Topic: MQTT_TOPIC_PREFIX + "+/api/+",
				QoS:   byte(qos),
			},
			{
				Topic: MQTT_TOPIC_PREFIX + "+/controller/+",
				QoS:   byte(qos),
			},
			{
				Topic: MQTT_TOPIC_PREFIX + "+/status/+",
				QoS:   byte(qos),
			},
			{
				Topic: MQTT_TOPIC_PREFIX + "+/async/+",
				QoS:   byte(qos),
			},
		},
	}); err != nil {
		log.Fatalln(err)
	}

	log.Printf("Subscribed to %s", MQTT_TOPIC_PREFIX+"+/controller/+")
	log.Printf("Subscribed to %s", MQTT_TOPIC_PREFIX+"+/status/+")
	log.Printf("Subscribed to %s", MQTT_TOPIC_PREFIX+"+/api/+")
	log.Printf("Subscribed to %s", MQTT_TOPIC_PREFIX+"+/async/+")
}

func buildClientConfig(status, controller, apiMsg, asyncMsg chan *paho.Publish, id string) *paho.ClientConfig {
	log.Println("Starting new MQTT client")
	singleHandler := paho.NewSingleHandlerRouter(func(p *paho.Publish) {

		if strings.Contains(p.Topic, "status") {
			status <- p
		} else if strings.Contains(p.Topic, "controller") {
			controller <- p
		} else if strings.Contains(p.Topic, "api") {
			apiMsg <- p
		} else if strings.Contains(p.Topic, "async") {
			asyncMsg <- p
		} else {
			log.Println("No handler for topic: ", p.Topic)
		}

	})

	clientConfig := paho.ClientConfig{}

	clientConfig = paho.ClientConfig{
		Router: singleHandler,
		OnServerDisconnect: func(d *paho.Disconnect) {
			if d.Properties != nil {
				log.Printf("Requested disconnect: %s\n , properties reason: %s\n", clientConfig.ClientID, d.Properties.ReasonString)
			} else {
				log.Printf("Requested disconnect; %s reason code: %d\n", clientConfig.ClientID, d.ReasonCode)
			}
		},
		OnClientError: func(err error) {
			log.Println(err)
		},
	}

	if id != "" {
		clientConfig.ClientID = id
	} else {
		clientConfig.ClientID = uuid.NewString()
	}

	return &clientConfig
}

func respondMsg(respond func(data []byte) error, code int, msgData any) {

	msg, err := json.Marshal(msgAnswer{
		Code: code,
		Msg:  msgData,
	})
	if err != nil {
		log.Printf("Failed to marshal message: %q", err)
		respond([]byte(err.Error()))
		return
	}

	respond([]byte(msg))
}

func tcpInfo(conn *net.TCPConn) (*unix.TCPInfo, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var info *unix.TCPInfo
	ctrlErr := raw.Control(func(fd uintptr) {
		info, err = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	})
	switch {
	case ctrlErr != nil:
		return nil, ctrlErr
	case err != nil:
		return nil, err
	}
	return info, nil
}

func (b *Bridge) setMqttPassword() {
	entry, err := b.kv.Get(b.Ctx, b.Mqtt.Username)
	if err != nil {
		log.Printf("Error getting key %s: %v", b.Mqtt.Username, err)
		return
	}

	b.Mqtt.Password = string(entry.Value())
}
