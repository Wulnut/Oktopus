package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/entity"
	local "github.com/leandrofars/oktopus/internal/nats"
	"github.com/leandrofars/oktopus/internal/utils"
)

type StatusCount struct {
	Online  int
	Offline int
}

type GeneralInfo struct {
	MqttRtt           string
	WebsocketsRtt     string
	StompRtt          string
	AcsRtt            string
	ProductClassCount []entity.ProductClassCount
	StatusCount       StatusCount
	VendorsCount      []entity.VendorsCount
}

func (a *Api) generalInfo(w http.ResponseWriter, r *http.Request) {

	var result GeneralInfo
	tenantSlug := middleware.GetTenantSlug(r)

	productclasscount, err := bridge.NatsReq[[]entity.ProductClassCount](
		local.NatsAdapterSubject(tenantSlug)+"devices.class",
		[]byte(""),
		w,
		a.nc,
	)
	if err != nil {
		return
	}

	vendorcount, err := bridge.NatsReq[[]entity.VendorsCount](
		local.NatsAdapterSubject(tenantSlug)+"devices.vendors",
		[]byte(""),
		w,
		a.nc,
	)
	if err != nil {
		return
	}

	statusCount, err := bridge.NatsReq[[]entity.StatusCount](
		local.NatsAdapterSubject(tenantSlug)+"devices.status",
		[]byte(""),
		w,
		a.nc,
	)
	if err != nil {
		return
	}

	for _, v := range statusCount.Msg {
		switch entity.Status(v.Status) {
		case entity.Online:
			result.StatusCount.Online = v.Count
		case entity.Offline:
			result.StatusCount.Offline = v.Count
		}
	}

	result.VendorsCount = vendorcount.Msg
	result.ProductClassCount = productclasscount.Msg

	// Run RTT checks concurrently to avoid sequential timeouts
	var wg sync.WaitGroup
	var mu sync.Mutex

	rttCheck := func(subject string, setter func(string)) {
		defer wg.Done()
		now := time.Now()
		_, err := bridge.NatsReqWithoutHttpSet[time.Duration](subject, []byte(""), a.nc)
		if err == nil {
			mu.Lock()
			setter(time.Since(now).String())
			mu.Unlock()
		}
	}

	wg.Add(4)
	go rttCheck(local.NatsWsAdapterSubjectPrefix(tenantSlug)+"rtt", func(v string) { result.WebsocketsRtt = v })
	go rttCheck(local.NatsCwmpAdapterSubjectPrefix(tenantSlug)+"rtt", func(v string) { result.AcsRtt = v })
	go rttCheck(local.NatsStompAdapterSubjectPrefix(tenantSlug)+"rtt", func(v string) { result.StompRtt = v })
	go rttCheck(local.NatsMqttAdapterSubjectPrefix(tenantSlug)+"rtt", func(v string) { result.MqttRtt = v })
	wg.Wait()

	err = json.NewEncoder(w).Encode(result)
	if err != nil {
		log.Println(err)
	}
}

func (a *Api) vendorsInfo(w http.ResponseWriter, r *http.Request) {
	vendors, err := bridge.NatsReq[[]entity.VendorsCount](
		local.NatsAdapterSubject(middleware.GetTenantSlug(r))+"devices.vendors",
		[]byte(""),
		w,
		a.nc,
	)
	if err != nil {
		return
	}
	utils.MarshallEncoder(vendors.Msg, w)
}

func (a *Api) productClassInfo(w http.ResponseWriter, r *http.Request) {
	vendors, err := bridge.NatsReq[[]entity.ProductClassCount](
		local.NatsAdapterSubject(middleware.GetTenantSlug(r))+"devices.class",
		[]byte(""),
		w,
		a.nc,
	)
	if err != nil {
		return
	}
	utils.MarshallEncoder(vendors.Msg, w)
}

func (a *Api) statusInfo(w http.ResponseWriter, r *http.Request) {
	vendors, err := bridge.NatsReq[[]entity.StatusCount](
		local.NatsAdapterSubject(middleware.GetTenantSlug(r))+"devices.status",
		[]byte(""),
		w,
		a.nc,
	)
	if err != nil {
		return
	}

	var status StatusCount
	for _, v := range vendors.Msg {
		switch entity.Status(v.Status) {
		case entity.Online:
			status.Online = v.Count
		case entity.Offline:
			status.Offline = v.Count
		}
	}

	utils.MarshallEncoder(status, w)
}
