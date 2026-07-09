package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/leandrofars/oktopus/internal/api"
	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/config"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/nats"
	"github.com/leandrofars/oktopus/internal/usp"
)

func main() {
	done := make(chan os.Signal, 1)

	signal.Notify(done, syscall.SIGINT)

	c := config.NewConfig()

	js, nc := nats.StartNatsClient(c.Nats)

	bridge := bridge.NewBridge(js, nc)

	database := db.NewDatabase(c.Mongo.Ctx, c.Mongo.Uri)

	// Idempotent migration: ensures every existing tenant has up-to-date indexes
	// (e.g. case-insensitive collation on the campaigns unique index).
	database.MigrateAllTenantIndexes(c.Mongo.Ctx)

	// Recreate missing per-tenant NATS KV buckets (device auth, CWMP conn-rq).
	if tenants, err := database.FindAllTenants(c.Mongo.Ctx); err != nil {
		log.Printf("EnsureTenantKVBuckets: list tenants: %v", err)
	} else {
		var slugs []string
		for _, t := range tenants {
			if t.Slug != "" && t.Status == db.TenantStatusActive {
				slugs = append(slugs, t.Slug)
			}
		}
		nats.EnsureTenantKVBuckets(c.Mongo.Ctx, js, slugs)
	}

	// Start message interceptor — resolves tenant DB dynamically from NATS subject
	usp.StartMessageInterceptor(c.Mongo.Ctx, nc, &database, c.Controller.ControllerId)

	a := api.NewApi(c, js, nc, bridge, database)
	api.InitLockScaleAdapters(c.LockScale)
	a.StartApi()
	a.InitLockEngine(c.LockCircuitBreaker)
	a.StartLockEngine()
	a.StartLockRetryScheduler()
	a.StartCampaignEngine()
	a.StartCampaignScheduler()

	<-done

	log.Println("rest api is shutting down...")
}
