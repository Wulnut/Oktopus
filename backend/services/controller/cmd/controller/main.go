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

	// Start message interceptor BEFORE API starts (to capture all messages)
	// Use a default tenant DB for the interceptor until tenant-scoped NATS subjects are implemented
	defaultTenantDB := database.ForTenant("default")
	usp.StartMessageInterceptor(c.Mongo.Ctx, nc, defaultTenantDB, c.Controller.ControllerId)

	a := api.NewApi(c, js, nc, bridge, database)
	a.StartApi()
	a.StartCampaignEngine()

	<-done

	log.Println("rest api is shutting down...")
}
