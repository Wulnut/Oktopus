package bridge

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/oktopUSP/oktopus/backend/services/mtp/stomp-adapter/internal/config"
	"github.com/oktopUSP/oktopus/backend/services/mtp/stomp-adapter/internal/stomp"
	"github.com/oktopUSP/oktopus/backend/services/mtp/stomp-adapter/internal/stomp/frame"
	"golang.org/x/sys/unix"
)

const (
	OFFLINE = iota
	ONLINE
)

const STOMP_CONNECTION_RETRY = 5 * time.Second

type msgAnswer struct {
	Code int
	Msg  any
}

const (
	NATS_STOMP_SUBJECT_PREFIX         = "stomp.usp.v1."
	NATS_STOMP_ADAPTER_SUBJECT_PREFIX = "stomp-adapter.usp.v1."
	DEVICE_SUBJECT_PREFIX             = "device.usp.v1."
	STOMP_QUEUE_PREFIX                = "oktopus/usp/v1/"
	STOMP_STATUS_QUEUE                = STOMP_QUEUE_PREFIX + "status"
	DEVICE_TIMEOUT_RESPONSE           = 5 * time.Second
	USP_CONTENT_TYPE                  = "application/vnd.bbf.usp.msg"
	DEFAULT_TENANT                    = "default"
)

type (
	Publisher  func(string, []byte) error
	Subscriber func(string, func(*nats.Msg)) error
)

type Bridge struct {
	Pub            Publisher
	Sub            Subscriber
	Stomp          config.Stomp
	Ctx            context.Context
	conn           *stomp.Conn
	asyncSubs      map[string]*stomp.Subscription // device -> persistent subscription for async messages
	asyncSubsMu    sync.RWMutex                   // mutex for asyncSubs
	onlineDevices  map[string]string              // device -> tenant slug (tracks online devices for reconnect)
	onlineDevicesMu sync.RWMutex
}

func NewBridge(p Publisher, s Subscriber, ctx context.Context, stompConfig config.Stomp) *Bridge {
	return &Bridge{
		Pub:           p,
		Sub:           s,
		Stomp:         stompConfig,
		Ctx:           ctx,
		asyncSubs:     make(map[string]*stomp.Subscription),
		onlineDevices: make(map[string]string),
	}
}

func (b *Bridge) StartBridge() {

	options := []func(*stomp.Conn) error{
		stomp.ConnOpt.Login(b.Stomp.User, b.Stomp.Password),
		stomp.ConnOpt.Host("/"),
	}

	var conn *stomp.Conn
	var err error

	go func() {
		for {
			conn, err = connectToServer(b.Stomp.Url, options)
			if err != nil {
				continue
			}
			b.conn = conn
			// Clean up stale subscriptions, then recreate for known online devices
			b.cleanupAllAsyncSubscriptions()
			b.recreateAsyncSubscriptions(conn)
			b.subscribe(conn)

			sub, err := conn.Subscribe(STOMP_STATUS_QUEUE, stomp.AckAuto)
			if err != nil {
				log.Println("cannot subscribe to", STOMP_STATUS_QUEUE, err.Error())
				return
			}
			log.Println("Subscribed to", STOMP_STATUS_QUEUE)

			for {
				if !sub.Active() {
					log.Println("Subscription is no longer active")
					break
				}
				msg := <-sub.C
				body := msg.Header.Get("message")
				if body != "connection closed" {
					log.Println("Received message", body)
					fmtBody := strings.Split(body, "|")
					if len(fmtBody) == 2 {
						deviceQueue := strings.Split(fmtBody[0], "/")
						device := deviceQueue[len(deviceQueue)-1]
						tenant := extractTenantFromDestination(fmtBody[0])
						status := fmtBody[1]
						log.Printf("[STOMP] Device status update: device=%s, tenant=%s, status=%s", device, tenant, status)
						b.Pub(NATS_STOMP_SUBJECT_PREFIX+tenant+"."+device+".status", []byte(status))

						// Handle persistent subscriptions based on device status
						if status == "1" {
							b.onlineDevicesMu.Lock()
							b.onlineDevices[device] = tenant
							b.onlineDevicesMu.Unlock()
							b.createAsyncSubscription(device, tenant, conn)
						} else if status == "0" {
							b.onlineDevicesMu.Lock()
							delete(b.onlineDevices, device)
							b.onlineDevicesMu.Unlock()
							b.removeAsyncSubscription(device)
						}
					} else {
						log.Printf("[STOMP] WARNING: Invalid status message format: %s", body)
					}
				}
			}
		}
	}()

}

func connectToServer(url string, options []func(*stomp.Conn) error) (*stomp.Conn, error) {

	conn, err := stomp.Dial("tcp", url, options...)

	if err != nil {
		log.Printf("Error to connect to %s, err: %s", url, err)
		time.Sleep(STOMP_CONNECTION_RETRY)
	} else {
		log.Println("Connected to STOMP server", url)
	}

	return conn, err
}

func (b *Bridge) subscribe(st *stomp.Conn) {

	b.Sub(NATS_STOMP_ADAPTER_SUBJECT_PREFIX+"*.*.info", func(msg *nats.Msg) {

		log.Printf("Received message on info subject")

		subj := strings.Split(msg.Subject, ".")
		device := subj[len(subj)-2]
		tenant := extractTenantFromSubject(msg.Subject)

		deviceInfoQueue := STOMP_QUEUE_PREFIX + tenant + "/controller/" + device + "/info"

		sub, err := st.Subscribe(deviceInfoQueue, stomp.AckAuto)
		if err != nil {
			log.Println("cannot subscribe to", deviceInfoQueue, err.Error())
			return
		}
		log.Println("Subscribed to", deviceInfoQueue)

		err = st.Send(STOMP_QUEUE_PREFIX+tenant+"/agent/"+device, "application/vnd.bbf.usp.msg", msg.Data, func(f *frame.Frame) error {
			f.Header.Set("reply-to-dest", deviceInfoQueue)
			return nil
		})

		if err != nil {
			log.Printf("send stomp msg error: %q", err)
			return
		}

		select {
		case data := <-sub.C:
			body := data.Body
			log.Println("Received message answer")
			err = b.Pub(NATS_STOMP_SUBJECT_PREFIX+tenant+"."+device+".info", body)
			if err != nil {
				log.Printf("send nats msg error: %q", err)
			}
		case <-time.After(DEVICE_TIMEOUT_RESPONSE):
			log.Println("Timeout waiting for device info response")
		}
		sub.Unsubscribe()
	})

	b.Sub(NATS_STOMP_ADAPTER_SUBJECT_PREFIX+"*.*.api", func(msg *nats.Msg) {

		log.Printf("[STOMP] Received message on NATS api subject: %s, size=%d bytes", msg.Subject, len(msg.Data))

		subj := strings.Split(msg.Subject, ".")
		device := subj[len(subj)-2]
		tenant := extractTenantFromSubject(msg.Subject)

		// Ensure async subscription exists for this device (handles adapter restart while device is online)
		b.onlineDevicesMu.Lock()
		b.onlineDevices[device] = tenant
		b.onlineDevicesMu.Unlock()
		b.createAsyncSubscription(device, tenant, st)

		deviceApiQueue := STOMP_QUEUE_PREFIX + tenant + "/controller/" + device + "/api"
		agentQueue := STOMP_QUEUE_PREFIX + tenant + "/agent/" + device

		log.Printf("[STOMP] Creating temporary subscription for device %s on queue %s", device, deviceApiQueue)
		sub, err := st.Subscribe(deviceApiQueue, stomp.AckAuto)
		if err != nil {
			log.Printf("[STOMP] ERROR: cannot subscribe to %s: %v", deviceApiQueue, err)
			return
		}
		log.Printf("[STOMP] SUCCESS: Subscribed to temporary queue %s (subscription ID: %s)", deviceApiQueue, sub.Id())

		log.Printf("[STOMP] Sending message to device %s on STOMP queue %s (reply-to: %s), size=%d bytes", device, agentQueue, deviceApiQueue, len(msg.Data))
		err = st.Send(agentQueue, "application/vnd.bbf.usp.msg", msg.Data, func(f *frame.Frame) error {
			f.Header.Set("reply-to-dest", deviceApiQueue)
			return nil
		})

		if err != nil {
			log.Printf("[STOMP] ERROR: send stomp msg error: %q", err)
			return
		}
		log.Printf("[STOMP] SUCCESS: Sent message to device %s on STOMP queue %s", device, agentQueue)

		select {
		case data := <-sub.C:
			body := data.Body
			log.Printf("[STOMP] Received response on temporary subscription for device %s (queue: %s), size=%d bytes", device, deviceApiQueue, len(body))
			err = b.Pub(DEVICE_SUBJECT_PREFIX+tenant+"."+device+".api", body)
			if err != nil {
				log.Printf("[STOMP] ERROR: send nats msg error: %q", err)
			} else {
				log.Printf("[STOMP] SUCCESS: Published response to NATS subject %s", DEVICE_SUBJECT_PREFIX+tenant+"."+device+".api")
			}
		case <-time.After(DEVICE_TIMEOUT_RESPONSE):
			log.Printf("[STOMP] WARNING: Timeout waiting for device %s response on queue %s", device, deviceApiQueue)
		}
		log.Printf("[STOMP] Unsubscribing from temporary subscription for device %s (queue: %s)", device, deviceApiQueue)
		sub.Unsubscribe()
	})

	b.Sub(NATS_STOMP_ADAPTER_SUBJECT_PREFIX+"*.rtt", func(msg *nats.Msg) {

		log.Printf("Received message on rtt subject")

		conn, err := net.Dial("tcp", b.Stomp.Url)
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

func extractTenantFromSubject(subject string) string {
	paths := strings.Split(subject, ".")
	if len(paths) >= 5 {
		return paths[3]
	}
	return DEFAULT_TENANT
}

// extractTenantFromDestination extracts tenant slug from STOMP destination path.
// New format: oktopus/usp/v1/<tenant>/agent/<endpoint_id> (6 parts, tenant at index 3)
// Old format: oktopus/usp/v1/agent/<endpoint_id> (5 parts, no tenant)
func extractTenantFromDestination(destination string) string {
	parts := strings.Split(destination, "/")
	// New format has 6+ parts with tenant at index 3
	if len(parts) >= 6 && (parts[4] == "agent" || parts[4] == "controller") {
		return parts[3]
	}
	// Old format or unknown — fall back to default
	return DEFAULT_TENANT
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

// createAsyncSubscription creates a persistent subscription to receive async messages (STOMPConnect, MQTTConnect, Disconnect, NOTIFY) from a device
func (b *Bridge) createAsyncSubscription(device, tenant string, conn *stomp.Conn) {
	b.asyncSubsMu.Lock()
	defer b.asyncSubsMu.Unlock()

	// Check if subscription already exists
	if _, exists := b.asyncSubs[device]; exists {
		return
	}

	asyncQueue := STOMP_QUEUE_PREFIX + tenant + "/controller/" + device + "/async"
	log.Printf("[STOMP] Creating async subscription for device %s on queue %s", device, asyncQueue)
	sub, err := conn.Subscribe(asyncQueue, stomp.AckAuto)
	if err != nil {
		log.Printf("[STOMP] ERROR: Failed to create async subscription for device %s: %v", device, err)
		return
	}

	b.asyncSubs[device] = sub
	log.Printf("[STOMP] Async subscription created for device %s", device)
	go b.handleAsyncMessages(device, tenant, sub)
}

// removeAsyncSubscription removes the persistent async subscription for a device
func (b *Bridge) removeAsyncSubscription(device string) {
	b.asyncSubsMu.Lock()
	defer b.asyncSubsMu.Unlock()
	
	sub, exists := b.asyncSubs[device]
	if !exists {
		return
	}
	
	if err := sub.Unsubscribe(); err != nil {
		log.Printf("[STOMP] ERROR: Failed to unsubscribe async subscription for device %s: %v", device, err)
	}
	
	delete(b.asyncSubs, device)
}

// handleAsyncMessages handles incoming async messages (STOMPConnect, MQTTConnect, Disconnect, NOTIFY) from a device's persistent subscription
func (b *Bridge) handleAsyncMessages(device, tenant string, sub *stomp.Subscription) {
	for {
		if !sub.Active() {
			b.asyncSubsMu.Lock()
			delete(b.asyncSubs, device)
			b.asyncSubsMu.Unlock()
			return
		}
		
		select {
		case msg, ok := <-sub.C:
			if !ok {
				b.asyncSubsMu.Lock()
				delete(b.asyncSubs, device)
				b.asyncSubsMu.Unlock()
				return
			}
			
			log.Printf("[STOMP] Received async message from device %s, size=%d bytes", device, len(msg.Body))
			err := b.Pub(NATS_STOMP_SUBJECT_PREFIX+tenant+"."+device+".async", msg.Body)
			if err != nil {
				log.Printf("[STOMP] ERROR: Failed to publish async message for device %s: %v", device, err)
			}
			
		case <-b.Ctx.Done():
			return
		}
	}
}

// cleanupAllAsyncSubscriptions removes all persistent async subscriptions (called on connection loss)
func (b *Bridge) cleanupAllAsyncSubscriptions() {
	b.asyncSubsMu.Lock()
	defer b.asyncSubsMu.Unlock()

	for device, sub := range b.asyncSubs {
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("[STOMP] ERROR: Failed to unsubscribe device %s during cleanup: %v", device, err)
		}
	}

	b.asyncSubs = make(map[string]*stomp.Subscription)
}

// recreateAsyncSubscriptions re-establishes async subscriptions for all known online devices after reconnect
func (b *Bridge) recreateAsyncSubscriptions(conn *stomp.Conn) {
	b.onlineDevicesMu.RLock()
	devices := make(map[string]string, len(b.onlineDevices))
	for device, tenant := range b.onlineDevices {
		devices[device] = tenant
	}
	b.onlineDevicesMu.RUnlock()

	if len(devices) == 0 {
		return
	}

	log.Printf("[STOMP] Recreating async subscriptions for %d online devices after reconnect", len(devices))
	for device, tenant := range devices {
		b.createAsyncSubscription(device, tenant, conn)
	}
}
