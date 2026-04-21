package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"os"
	"strings"

	"github.com/go-stomp/stomp/v3/server"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NatsAuthenticator validates STOMP credentials against per-tenant NATS KV buckets.
// Username format: <tenant_slug>/<endpoint_id>
// Checks per-device credential first, then falls back to shared tenant password.
type NatsAuthenticator struct {
	js jetstream.JetStream
}

func (a *NatsAuthenticator) Authenticate(login, passcode string) bool {
	log.Printf("auth: attempt login=%q passcode=[REDACTED]", login)
	if login == "" {
		log.Println("auth: empty login rejected")
		return false
	}

	if a.js == nil {
		log.Println("auth: NATS not connected, rejecting all auth attempts")
		return false
	}

	// Parse tenant/device from login
	parts := strings.SplitN(login, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		log.Printf("auth: invalid login format (expected tenant/device): %s", login)
		return false
	}
	tenant := parts[0]

	bucketName := "devices-auth-" + tenant
	kv, err := a.js.KeyValue(context.Background(), bucketName)
	if err != nil {
		log.Printf("auth: KV bucket %s not found: %v", bucketName, err)
		return false
	}

	// Try per-device credential first (sanitize key: colons not allowed in NATS KV keys)
	kvKey := strings.ReplaceAll(login, ":", "_")
	entry, err := kv.Get(context.Background(), kvKey)
	if err == nil && string(entry.Value()) == passcode {
		return true
	}

	// Fall back to shared tenant password
	entry, err = kv.Get(context.Background(), "__tenant_password__")
	if err == nil && string(entry.Value()) == passcode {
		return true
	}

	log.Printf("auth: failed for user %s", login)
	return false
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading godotenv file")
	}
	localEnv := ".env.local"
	if _, err = os.Stat(localEnv); err == nil {
		_ = godotenv.Overload(localEnv)
		log.Println("Loaded variables from '.env.local'")
	} else {
		log.Println("Loaded variables from '.env'")
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Connect to NATS for credential lookups
	var js jetstream.JetStream
	natsURL := os.Getenv("NATS_URL")
	if natsURL != "" {
		opts := []nats.Option{
			nats.Name("stomp-server"),
			nats.MaxReconnects(-1),
		}

		enableTLS := strings.EqualFold(os.Getenv("NATS_ENABLE_TLS"), "true")
		if enableTLS {
			clientCrt := os.Getenv("CLIENT_CRT")
			clientKey := os.Getenv("CLIENT_KEY")
			serverCA := os.Getenv("SERVER_CA")

			if serverCA != "" {
				opts = append(opts, nats.RootCAs(serverCA))
			}
			if clientCrt != "" && clientKey != "" {
				cert, err := tls.LoadX509KeyPair(clientCrt, clientKey)
				if err != nil {
					log.Printf("Warning: failed to load client cert: %v", err)
				} else {
					tlsConfig := &tls.Config{
						Certificates: []tls.Certificate{cert},
					}
					if serverCA != "" {
						caCert, err := os.ReadFile(serverCA)
						if err == nil {
							pool := x509.NewCertPool()
							pool.AppendCertsFromPEM(caCert)
							tlsConfig.RootCAs = pool
						}
					}
					opts = append(opts, nats.Secure(tlsConfig))
				}
			}
		}

		nc, err := nats.Connect(natsURL, opts...)
		if err != nil {
			log.Printf("Warning: failed to connect to NATS at %s: %v (auth will reject all)", natsURL, err)
		} else {
			log.Printf("Connected to NATS at %s", natsURL)
			jsCtx, err := jetstream.New(nc)
			if err != nil {
				log.Printf("Warning: failed to create JetStream client: %v (auth will reject all)", err)
			} else {
				js = jsCtx
			}
		}
	} else {
		log.Println("Warning: NATS_URL not set, STOMP auth will reject all connections")
	}

	auth := &NatsAuthenticator{js: js}

	l, err := net.Listen("tcp", server.DefaultAddr)
	if err != nil {
		log.Println("Error to open tcp port: ", err)
	}

	s := server.Server{
		Addr:          server.DefaultAddr,
		HeartBeat:     server.DefaultHeartBeat,
		Authenticator: auth,
	}

	log.Println("Started STOMP server at port", s.Addr)
	err = s.Serve(l)
	if err != nil {
		log.Println("Error to start stomp server: ", err)
	}
}
