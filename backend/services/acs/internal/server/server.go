package server

import (
	"log"
	"net/http"
	"oktopUSP/backend/services/acs/internal/config"
	"oktopUSP/backend/services/acs/internal/nats"
	"oktopUSP/backend/services/acs/internal/server/handler"
	"strings"
)

func Run(c config.Acs, natsActions nats.NatsActions, h *handler.Handler) {

	route := strings.TrimSuffix(c.Route, "/")
	if route == "" {
		route = "/acs"
	}

	mux := http.NewServeMux()
	mux.HandleFunc(route+"/", h.CwmpHandler)
	mux.HandleFunc(route, h.CwmpHandler)
	go h.HandleCpeStatus()

	log.Printf("ACS running at %s (CWMP routes %s and %s/{tenant}/)", c.Port, route, route)

	err := http.ListenAndServe(c.Port, mux)
	if err != nil {
		log.Fatal(err)
	}
}
