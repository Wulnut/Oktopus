package nats

import (
	"log"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/oktopUSP/oktopus/backend/services/mtp/stomp-adapter/internal/config"
)

const (
	STREAM_NAME = "stomp"
)

func StartNatsClient(c config.Nats) (
	*nats.Conn,
	func(string, []byte) error,
	func(string, func(*nats.Msg)) error,
) {

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

	return nc, publisher(js, nc), subscriber(nc)
}

func subscriber(nc *nats.Conn) func(string, func(*nats.Msg)) error {
	return func(subject string, handler func(*nats.Msg)) error {
		_, err := nc.Subscribe(subject, handler)
		if err != nil {
			log.Printf("error to subscribe to subject %s error: %q", subject, err)
		}
		return err
	}
}

func publisher(js jetstream.JetStream, nc *nats.Conn) func(string, []byte) error {
	return func(subject string, payload []byte) error {
		// Use regular NATS publish for device.usp.v1.* subjects (request/response, not persistent)
		// These subjects don't match any JetStream stream, so using JetStream causes queue stalls
		// The controller subscribes to these via regular NATS (nc.ChanSubscribe), so regular publish works
		if strings.HasPrefix(subject, "device.usp.v1.") {
			err := nc.Publish(subject, payload)
			if err != nil {
				log.Printf("error to send nats message: %q", err)
			}
			return err
		}
		// Use JetStream for other subjects that match streams (stomp.usp.v1.*, etc.)
		_, err := js.PublishAsync(subject, payload)
		if err != nil {
			log.Printf("error to send jetstream message: %q", err)
		}
		return err
	}
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
