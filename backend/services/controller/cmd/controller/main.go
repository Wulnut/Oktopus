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

	js, nc, kv := nats.StartNatsClient(c.Nats)

	bridge := bridge.NewBridge(js, nc)

	db := db.NewDatabase(c.Mongo.Ctx, c.Mongo.Uri)

	// Start message interceptor BEFORE API starts (to capture all messages)
	usp.StartMessageInterceptor(c.Mongo.Ctx, nc, db, c.Controller.ControllerId)

	api := api.NewApi(c, js, nc, bridge, db, kv)
	api.StartApi()
	api.StartCampaignEngine()

	<-done

	log.Println("rest api is shutting down...")
}
