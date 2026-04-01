package nats

import (
	"context"
	"log"
	"time"

	"github.com/leandrofars/oktopus/internal/config"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	NATS_ACCOUNT_SUBJ_PREFIX = "account-manager.v1."
	NATS_REQUEST_TIMEOUT     = 10 * time.Second
)

// Tenant-scoped NATS subject prefix functions.
// Each inserts the tenant slug into the subject for message isolation between tenants.

func NatsMqttSubjectPrefix(tenantSlug string) string {
	return "mqtt.usp.v1." + tenantSlug + "."
}

func NatsMqttAdapterSubjectPrefix(tenantSlug string) string {
	return "mqtt-adapter.usp.v1." + tenantSlug + "."
}

func NatsAdapterSubject(tenantSlug string) string {
	return "adapter.usp.v1." + tenantSlug + "."
}

func NatsWsSubjectPrefix(tenantSlug string) string {
	return "ws.usp.v1." + tenantSlug + "."
}

func NatsWsAdapterSubjectPrefix(tenantSlug string) string {
	return "ws-adapter.usp.v1." + tenantSlug + "."
}

func NatsStompAdapterSubjectPrefix(tenantSlug string) string {
	return "stomp-adapter.usp.v1." + tenantSlug + "."
}

func DeviceSubjectPrefix(tenantSlug string) string {
	return "device.usp.v1." + tenantSlug + "."
}

func DeviceCwmpSubjectPrefix(tenantSlug string) string {
	return "device.cwmp.v1." + tenantSlug + "."
}

func NatsCwmpAdapterSubjectPrefix(tenantSlug string) string {
	return "cwmp-adapter.v1." + tenantSlug + "."
}

func StartNatsClient(c config.Nats) (jetstream.JetStream, *nats.Conn) {

	var (
		nc  *nats.Conn
		err error
	)

	opts := defineOptions(c)

	log.Printf("Connecting to NATS server %s", c.Url)

	for {
		nc, err = nats.Connect(c.Url, opts...)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}
	log.Printf("Successfully connected to NATS server %s", c.Url)

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Failed to create JetStream client: %v", err)
	}

	return js, nc
}

// CreateTenantKVBucket creates or updates a NATS KeyValue bucket for tenant device authentication.
func CreateTenantKVBucket(js jetstream.JetStream, slug string) (jetstream.KeyValue, error) {
	return js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:      "devices-auth-" + slug,
		Description: "Device authentication for tenant " + slug,
	})
}

// DeleteTenantKVBucket deletes a tenant's device authentication KV bucket.
func DeleteTenantKVBucket(js jetstream.JetStream, slug string) error {
	return js.DeleteKeyValue(context.Background(), "devices-auth-"+slug)
}

func defineOptions(c config.Nats) []nats.Option {
	var opts []nats.Option

	opts = append(opts, nats.Name(c.Name))
	opts = append(opts, nats.MaxReconnects(-1))
	opts = append(opts, nats.ReconnectWait(5*time.Second))
	opts = append(opts, nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
		log.Printf("Got disconnected! Reason: %q\n", err)
	}))
	opts = append(opts, nats.ReconnectHandler(func(nc *nats.Conn) {
		log.Printf("Got reconnected to %v!\n", nc.ConnectedUrl())
	}))
	opts = append(opts, nats.ClosedHandler(func(nc *nats.Conn) {
		log.Printf("Connection closed. Reason: %q\n", nc.LastError())
	}))
	if c.EnableTls {
		log.Printf("Load certificates: %s and %s\n", c.Cert.CertFile, c.Cert.KeyFile)
		opts = append(opts, nats.RootCAs(c.Cert.CaFile))
		opts = append(opts, nats.ClientCert(c.Cert.CertFile, c.Cert.KeyFile))
	}

	return opts
}
