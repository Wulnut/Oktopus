package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/leandrofars/oktopus/internal/api/cors"
	"github.com/leandrofars/oktopus/internal/api/middleware"
	"github.com/leandrofars/oktopus/internal/bridge"
	"github.com/leandrofars/oktopus/internal/config"
	"github.com/leandrofars/oktopus/internal/db"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Api struct {
	port                      string
	js                        jetstream.JetStream
	nc                        *nats.Conn
	bridge                    bridge.Bridge
	db                        db.Database
	ctx                       context.Context
	campaignSchedulerEnabled  bool
	campaignSchedulerInterval time.Duration
	lockRetryEnabled          bool
	lockRetryInterval         time.Duration
	lockCommandTimeout        time.Duration
	lockMaxAttempts           int
}

const REQUEST_TIMEOUT = time.Second * 30

func NewApi(c *config.Config, js jetstream.JetStream, nc *nats.Conn, bridge bridge.Bridge, d db.Database) Api {
	return Api{
		port:                      c.RestApi.Port,
		js:                        js,
		nc:                        nc,
		ctx:                       c.RestApi.Ctx,
		bridge:                    bridge,
		db:                        d,
		campaignSchedulerEnabled:  c.CampaignScheduler.Enabled,
		campaignSchedulerInterval: c.CampaignScheduler.Interval,
		lockRetryEnabled:          c.LockRetryScheduler.Enabled,
		lockRetryInterval:         c.LockRetryScheduler.Interval,
		lockCommandTimeout:        c.LockRetryScheduler.CommandTimeout,
		lockMaxAttempts:           c.LockRetryScheduler.MaxAttempts,
	}
}

// tenantDB returns a TenantDB scoped to the tenant from the request context.
func (a *Api) tenantDB(r *http.Request) *db.TenantDB {
	slug := middleware.GetTenantSlug(r)
	return a.db.ForTenant(slug)
}

// tenantKV returns the NATS KeyValue bucket for device auth scoped to the tenant from the request context.
func (a *Api) tenantKV(r *http.Request) (jetstream.KeyValue, error) {
	slug := middleware.GetTenantSlug(r)
	return a.js.KeyValue(a.ctx, "devices-auth-"+slug)
}

func (a *Api) StartApi() {
	r := mux.NewRouter()
	setJSONNotFoundHandler(r)

	/* ----- Auth routes (no middleware) ----- */
	authentication := r.PathPrefix("/api/auth").Subrouter()
	authentication.HandleFunc("/login", a.generateToken).Methods("PUT")
	authentication.HandleFunc("/admin/register", a.registerAdminUser).Methods("POST")
	authentication.HandleFunc("/admin/exists", a.adminUserExists).Methods("GET")

	/* ----- Tenant management routes (AuthMiddleware only, SuperAdmin) ----- */
	tenants := r.PathPrefix("/api/tenants").Subrouter()
	tenants.Use(middleware.AuthMiddleware)
	tenants.HandleFunc("", a.createTenant).Methods("POST")
	tenants.HandleFunc("", a.listTenants).Methods("GET")
	tenants.HandleFunc("/{slug}", a.getTenant).Methods("GET")
	tenants.HandleFunc("/{slug}", a.updateTenant).Methods("PUT")
	tenants.HandleFunc("/{slug}", a.deleteTenant).Methods("DELETE")

	/* ----- All tenant-scoped data routes ----- */
	tenantRouter := r.PathPrefix("/api/tenants/{slug}").Subrouter()
	tenantRouter.Use(middleware.AuthMiddleware)
	tenantRouter.Use(middleware.TenantMiddleware(func(slug string) (interface{}, error) {
		return a.db.FindTenant(context.Background(), slug)
	}))

	// Device password (shared tenant credential)
	tenantRouter.HandleFunc("/device-password", a.getDevicePassword).Methods("GET")
	tenantRouter.HandleFunc("/device-password", a.setDevicePassword).Methods("PUT")

	// CA Certs
	tenantRouter.HandleFunc("/ca-certs", a.listCACerts).Methods("GET")
	tenantRouter.HandleFunc("/ca-certs", a.addCACert).Methods("POST")
	tenantRouter.HandleFunc("/ca-certs/{certId}", a.removeCACert).Methods("DELETE")

	// Users
	tenantRouter.HandleFunc("/users", a.retrieveUsers).Methods("GET")
	tenantRouter.HandleFunc("/users", a.registerUser).Methods("POST")
	tenantRouter.HandleFunc("/users/{user}", a.deleteUser).Methods("DELETE")
	tenantRouter.HandleFunc("/users/password/{user}", a.changePassword).Methods("PUT")
	tenantRouter.HandleFunc("/users/password", a.changePassword).Methods("PUT")

	// Devices
	iot := tenantRouter.PathPrefix("/device").Subrouter()
	iot.HandleFunc("/alias", a.setDeviceAlias).Methods("PUT")
	iot.HandleFunc("/auth", a.deviceAuth).Methods("GET", "POST", "DELETE")
	iot.HandleFunc("/message/{type}", a.addTemplate).Methods("POST")
	iot.HandleFunc("/message", a.updateTemplate).Methods("PUT")
	iot.HandleFunc("/message", a.getTemplate).Methods("GET")
	iot.HandleFunc("/message", a.deleteTemplate).Methods("DELETE")
	iot.HandleFunc("/cwmp/{sn}/generic", a.cwmpGenericMsg).Methods("PUT")
	iot.HandleFunc("/cwmp/{sn}/getParameterNames", a.cwmpGetParameterNamesMsg).Methods("PUT")
	iot.HandleFunc("/cwmp/{sn}/getParameterValues", a.cwmpGetParameterValuesMsg).Methods("PUT")
	iot.HandleFunc("/cwmp/{sn}/getParameterAttributes", a.cwmpGetParameterAttributesMsg).Methods("PUT")
	iot.HandleFunc("/cwmp/{sn}/setParameterValues", a.cwmpSetParameterValuesMsg).Methods("PUT")
	iot.HandleFunc("/cwmp/{sn}/addObject", a.cwmpAddObjectMsg).Methods("PUT")
	iot.HandleFunc("/cwmp/{sn}/deleteObject", a.cwmpDeleteObjectMsg).Methods("PUT")
	iot.HandleFunc("/cwmp/{sn}/root", a.cwmpRootGet).Methods("GET")
	iot.HandleFunc("/cwmp/{sn}/info", a.cwmpInfoGet).Methods("GET")
	iot.HandleFunc("/cwmp/{sn}/parameters", a.cwmpParametersBrowse).Methods("PUT")
	iot.HandleFunc("/cwmp/{sn}/set", a.cwmpSetValuesJson).Methods("PUT")
	iot.HandleFunc("", a.retrieveDevices).Methods("GET", "DELETE")
	iot.HandleFunc("/filterOptions", a.filterOptions).Methods("GET")
	iot.HandleFunc("/{sn}/{mtp}/generic", a.deviceGenericMessage).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/get", a.deviceGetMsg).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/add", a.deviceCreateMsg).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/del", a.deviceDeleteMsg).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/set", a.deviceUpdateMsg).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/notify", a.deviceNotifyMsg).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/parameters", a.deviceGetSupportedParametersMsg).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/instances", a.deviceGetParameterInstances).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/operate", a.deviceOperateMsg).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/fw_update", a.deviceFwUpdate).Methods("PUT")
	iot.HandleFunc("/{sn}/wifi", a.deviceWifi).Methods("PUT", "GET")
	iot.HandleFunc("/{sn}/history", a.deviceMessageHistory).Methods("GET")
	iot.HandleFunc("/{sn}/history", a.deviceClearHistory).Methods("DELETE")
	iot.HandleFunc("/{sn}/{mtp}/info", a.deviceInfoGet).Methods("GET")
	iot.HandleFunc("/{sn}/{mtp}/wifi-usp", a.deviceWifiUspGet).Methods("GET")
	iot.HandleFunc("/{sn}/{mtp}/interfaces", a.deviceInterfacesGet).Methods("GET")
	iot.HandleFunc("/{sn}/{mtp}/performance", a.devicePerformanceGet).Methods("GET")
	iot.HandleFunc("/{sn}/metrics", a.deviceMetricsHistory).Methods("GET")
	iot.HandleFunc("/{sn}/{mtp}/reboot", a.deviceReboot).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/factory-reset", a.deviceFactoryReset).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/restart-agent", a.deviceRestartAgent).Methods("PUT")
	iot.HandleFunc("/{sn}/{mtp}/topology", a.deviceTopology).Methods("GET")
	iot.HandleFunc("/{sn}/cached-info", a.deviceCachedInfoGet).Methods("GET")
	iot.HandleFunc("/{sn}/fw-policy", a.getDeviceFWPolicy).Methods("GET")
	iot.HandleFunc("/{sn}/fw-policy", a.setDeviceFWPolicy).Methods("PUT")
	iot.HandleFunc("/{sn}/upgrade-logs", a.deviceUpgradeLogs).Methods("GET")

	// Dashboard info
	dash := tenantRouter.PathPrefix("/info").Subrouter()
	dash.HandleFunc("/vendors", a.vendorsInfo).Methods("GET")
	dash.HandleFunc("/status", a.statusInfo).Methods("GET")
	dash.HandleFunc("/device_class", a.productClassInfo).Methods("GET")
	dash.HandleFunc("/general", a.generalInfo).Methods("GET")

	// Firmware
	firmware := tenantRouter.PathPrefix("/firmware").Subrouter()
	firmware.HandleFunc("", a.listFirmware).Methods("GET")
	firmware.HandleFunc("", a.uploadFirmware).Methods("POST")
	firmware.HandleFunc("/{id}", a.updateFirmware).Methods("PUT")
	firmware.HandleFunc("/{id}", a.deleteFirmware).Methods("DELETE")
	firmware.HandleFunc("/{id}/phase", a.updateFirmwarePhase).Methods("PUT")

	// Scripts
	scripts := tenantRouter.PathPrefix("/scripts").Subrouter()
	scripts.HandleFunc("", a.listScripts).Methods("GET")
	scripts.HandleFunc("", a.createScript).Methods("POST")
	scripts.HandleFunc("/{id}", a.getScript).Methods("GET")
	scripts.HandleFunc("/{id}", a.updateScript).Methods("PUT")
	scripts.HandleFunc("/{id}", a.deleteScript).Methods("DELETE")
	scripts.HandleFunc("/{id}/execute/{sn}/{mtp}", a.executeScriptHandler).Methods("POST")
	scripts.HandleFunc("/{id}/executions", a.listScriptExecutions).Methods("GET")
	scripts.HandleFunc("/{id}/executions/{execId}", a.getExecution).Methods("GET")

	// Campaigns
	campaigns := tenantRouter.PathPrefix("/campaigns").Subrouter()
	campaigns.HandleFunc("", a.listCampaigns).Methods("GET")
	campaigns.HandleFunc("", a.createCampaign).Methods("POST")
	campaigns.HandleFunc("/{id}", a.updateCampaign).Methods("PUT")
	campaigns.HandleFunc("/{id}", a.deleteCampaign).Methods("DELETE")
	campaigns.HandleFunc("/{id}/logs", a.campaignUpgradeLogs).Methods("GET")

	// ONT Lock / Device admission control
	lock := tenantRouter.PathPrefix("/lock").Subrouter()
	lock.HandleFunc("/policies", a.listLockPolicies).Methods("GET")
	lock.HandleFunc("/whitelist", a.upsertWhitelistPolicy).Methods("POST")
	lock.HandleFunc("/whitelist/batch", a.batchWhitelistPolicies).Methods("POST")
	lock.HandleFunc("/blacklist", a.upsertBlacklistPolicy).Methods("POST")
	lock.HandleFunc("/blacklist/batch", a.batchBlacklistPolicies).Methods("POST")
	lock.HandleFunc("/policies/{sn}", a.deleteLockPolicy).Methods("DELETE")
	lock.HandleFunc("/blacklist/{sn}", a.deleteLockPolicy).Methods("DELETE")
	lock.HandleFunc("/config", a.getLockConfig).Methods("GET")
	lock.HandleFunc("/config", a.updateLockConfig).Methods("PUT")
	lock.HandleFunc("/unauthorized", a.listUnauthorizedDevices).Methods("GET")
	lock.HandleFunc("/unauthorized/batch-whitelist", a.batchWhitelistFromUnauthorized).Methods("POST")
	lock.HandleFunc("/commands", a.listLockCommands).Methods("GET")
	lock.HandleFunc("/audit", a.listLockAuditLogs).Methods("GET")

	// Mass actions
	mass := tenantRouter.PathPrefix("/mass-actions").Subrouter()
	mass.HandleFunc("", a.listMassActions).Methods("GET")
	mass.HandleFunc("/script", a.massScriptExecution).Methods("POST")
	mass.HandleFunc("/{id}", a.getMassAction).Methods("GET")
	mass.HandleFunc("/{id}/cancel", a.cancelMassAction).Methods("POST")

	/* -------------------------------------------------------------------------- */

	corsOpts := cors.GetCorsConfig()

	srv := &http.Server{
		Addr:         "0.0.0.0:" + a.port,
		WriteTimeout: time.Second * 60,
		ReadTimeout:  time.Second * 60,
		IdleTimeout:  time.Second * 60,
		Handler:      corsOpts.Handler(r),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()
	log.Println("Running REST API at port", a.port)
}

func setJSONNotFoundHandler(r *mux.Router) {
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error":  "route not registered",
			"path":   req.URL.Path,
			"method": req.Method,
		})
	})
}
