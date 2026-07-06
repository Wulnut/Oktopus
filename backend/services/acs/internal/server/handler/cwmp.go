package handler

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"oktopUSP/backend/services/acs/internal/auth"
	"oktopUSP/backend/services/acs/internal/cwmp"
	"time"

	"github.com/oleiade/lane"
)

func (h *Handler) CwmpHandler(w http.ResponseWriter, r *http.Request) {

	log.Printf("--> Connection from %s", r.RemoteAddr)

	defer r.Body.Close()
	defer log.Printf("<-- Connection from %s closed", r.RemoteAddr)

	tmp, _ := ioutil.ReadAll(r.Body)
	body := string(tmp)

	if h.acsConfig.DebugMode {
		log.Println("Received message: ", body)
	}

	var envelope cwmp.SoapEnvelope
	xml.Unmarshal(tmp, &envelope)

	messageType := envelope.Body.CWMPMessage.XMLName.Local
	log.Println("messageType: ", messageType)

	var cpe CPE
	var exists bool

	w.Header().Set("Server", "Oktopus "+Version)

	tenantSlug := ParseTenantSlug(r.URL.Path, h.acsConfig.Route)

	// Empty probes without a CWMP session cookie are health checks, not TR-069 session polls.
	if messageType == "" && len(body) == 0 {
		if _, err := r.Cookie("oktopus"); err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	cookieSN := ""
	if messageType != "Inform" {
		if cookie, err := r.Cookie("oktopus"); err == nil {
			cookieSN = cookie.Value
			h.mu.RLock()
			cpe, exists = h.Cpes[cookieSN]
			h.mu.RUnlock()
			if !exists {
				log.Printf("CPE with serial number %s not found", cookieSN)
				w.WriteHeader(http.StatusNotFound)
				return
			} else {
				log.Printf("CPE with serial number %s found", cookieSN)
			}
		} else {
			log.Println("cookie 'oktopus' missing")
			w.WriteHeader(401)
			return
		}
	}

	if messageType == "Inform" {
		var Inform cwmp.CWMPInform
		xml.Unmarshal(tmp, &Inform)

		var addr string
		if r.Header.Get("X-Real-Ip") != "" {
			addr = r.Header.Get("X-Real-Ip")
		} else {
			addr = r.RemoteAddr
		}

		sn := Inform.DeviceId.SerialNumber

		h.mu.Lock()
		if existing, ok := h.Cpes[sn]; !ok {
			log.Println("New device: " + sn + " tenant: " + tenantSlug)
			cpe = CPE{
				SerialNumber:         sn,
				TenantSlug:           tenantSlug,
				LastConnection:       time.Now(),
				SoftwareVersion:      Inform.GetSoftwareVersion(),
				HardwareVersion:      Inform.GetHardwareVersion(),
				ExternalIPAddress:    addr,
				ConnectionRequestURL: Inform.GetConnectionRequest(),
				OUI:                  Inform.DeviceId.OUI,
				Queue:                lane.NewQueue(),
				DataModel:            Inform.GetDataModelType(),
			}
			h.Cpes[sn] = cpe
		} else {
			cpe = existing
			cpe.TenantSlug = tenantSlug
			cpe.LastConnection = time.Now()
			cpe.SoftwareVersion = Inform.GetSoftwareVersion()
			cpe.HardwareVersion = Inform.GetHardwareVersion()
			cpe.ExternalIPAddress = addr
			cpe.ConnectionRequestURL = Inform.GetConnectionRequest()
			cpe.OUI = Inform.DeviceId.OUI
			cpe.DataModel = Inform.GetDataModelType()
			h.Cpes[sn] = cpe
		}
		h.mu.Unlock()

		// Provision (or load) per-CPE Connection Request credentials. On KV
		// miss this also enqueues a SetParameterValues into the CPE session
		// queue, which the CPE will drain on its next empty POST. We do this
		// outside the h.mu critical section because the KV roundtrip can take
		// hundreds of ms on a busy NATS cluster.
		if user, pass, ok := h.ensureConnReqCreds(tenantSlug, sn, &cpe); ok {
			h.mu.Lock()
			if stored, exists := h.Cpes[sn]; exists {
				stored.Username = user
				stored.Password = pass
				h.Cpes[sn] = stored
				cpe = stored
			}
			h.mu.Unlock()
		}

		h.pub(cwmpInfoSubject(tenantSlug, sn), tmp)

		log.Printf("Received an Inform from device %s tenant %s withEventCodes %s", addr, tenantSlug, Inform.GetEvents())

		expiration := time.Now().AddDate(0, 0, 1)

		cookie := http.Cookie{Name: "oktopus", Value: sn, Expires: expiration}
		http.SetCookie(w, &cookie)
		//data, _ := xml.Marshal(cwmp.InformResponse(envelope.Header.Id))
		io.WriteString(w, cwmp.InformResponse(envelope.Header.Id))

	} else if messageType == "GetRPCMethods" || messageType == "GetRPC" {
		h.touchCPE(cookieSN)
		io.WriteString(w, cwmp.GetRPCMethodsResponse(envelope.Header.Id))
	} else if messageType == "TransferComplete" {

	} else {

		if len(body) == 0 {
			log.Println("Got Empty Post")
		}

		h.mu.Lock()
		cpe = h.Cpes[cookieSN]
		waiting := cpe.Waiting
		if waiting != nil {
			cpe.Waiting = nil
			h.Cpes[cookieSN] = cpe
		}
		h.mu.Unlock()

		if waiting != nil {

			log.Println("ACS was waiting for a response from the CPE, now received something")

			var e cwmp.SoapEnvelope
			xml.Unmarshal([]byte(body), &e)
			log.Println("Kind of envelope: ", e.KindOf())

			if e.KindOf() == "GetParameterNamesResponse" {
				log.Println("Receive GetParameterNamesResponse from CPE:", cpe.SerialNumber)
				msgAnswer(waiting.Callback, waiting.Time, h.acsConfig.DeviceAnswerTimeout, tmp)
			} else if e.KindOf() == "GetParameterValuesResponse" {
				log.Println("Receive GetParameterValuesResponse from CPE:", cpe.SerialNumber)
				msgAnswer(waiting.Callback, waiting.Time, h.acsConfig.DeviceAnswerTimeout, tmp)
			} else if e.KindOf() == "SetParameterValuesResponse" {
				log.Println("Receive SetParameterValuesResponse from CPE:", cpe.SerialNumber)
				msgAnswer(waiting.Callback, waiting.Time, h.acsConfig.DeviceAnswerTimeout, tmp)
			} else if e.KindOf() == "Fault" {
				log.Println("Receive FaultResponse from CPE:", cpe.SerialNumber)
				msgAnswer(waiting.Callback, waiting.Time, h.acsConfig.DeviceAnswerTimeout, tmp)
				log.Println(body)
			} else {
				log.Println("Unknown message type")
				log.Println("Body:", body)
				msgAnswer(waiting.Callback, waiting.Time, h.acsConfig.DeviceAnswerTimeout, tmp)
			}
		} else {
			log.Println("CPE was not waiting for a response")
		}

		h.mu.Lock()
		cpe, exists = h.Cpes[cookieSN]
		if !exists {
			h.mu.Unlock()
			w.WriteHeader(http.StatusNotFound)
			return
		}
		cpe.LastConnection = time.Now()
		log.Printf("CPE %s Queue size: %d", cpe.SerialNumber, cpe.Queue.Size())

		if cpe.Queue.Size() > 0 {
			req := cpe.Queue.Dequeue().(Request)
			cpe.Waiting = &req
			h.Cpes[cookieSN] = cpe
			h.mu.Unlock()
			log.Println("Sending request to CPE:", req.Id)
			w.Header().Set("Connection", "keep-alive")
			w.Write(req.CwmpMsg)
		} else {
			h.Cpes[cookieSN] = cpe
			h.mu.Unlock()
			w.Header().Set("Connection", "close")
			w.WriteHeader(204)
		}
	}
}

func (h *Handler) touchCPE(sn string) {
	if sn == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	cpe, ok := h.Cpes[sn]
	if !ok {
		return
	}
	cpe.LastConnection = time.Now()
	h.Cpes[sn] = cpe
}

// connectionRequestCreds returns the (username, password) the ACS should use
// when authenticating to a CPE's ConnectionRequestURL.
//
// Per TR-069 (BBF TR-069 Issue 1 Amendment 6, §3.2.2 + TR-181 §A.3.2.6) the
// CPE validates the ACS using the values of:
//
//	Device.ManagementServer.ConnectionRequestUsername
//	Device.ManagementServer.ConnectionRequestPassword
//
// Best practice is for the ACS to provision unique random credentials per CPE
// during the BOOTSTRAP Inform via SetParameterValues, persist them, then use
// them here. Until that is implemented, we fall back to the global
// CONN_RQ_USER / CONN_RQ_PASSWD pair from configuration.
//
// TODO(security): generate per-(tenant, SN) random credentials on first
// Inform, write them to the CPE via SetParameterValues, persist them in NATS
// KV / Mongo, and populate cpe.Username / cpe.Password from that store.
// The current shared-secret model means a single leaked credential pair lets
// any reachable client trigger Connection Requests against every CPE in the
// platform.
func (h *Handler) connectionRequestCreds(cpe CPE) (string, string) {
	user := cpe.Username
	if user == "" {
		user = h.acsConfig.ConnReqUsername
	}
	pass := cpe.Password
	if pass == "" {
		pass = h.acsConfig.ConnReqPassword
	}
	return user, pass
}

func (h *Handler) ConnectionRequest(cpe CPE) error {
	log.Println("--> ConnectionRequest, CPE: ", cpe.SerialNumber)

	user, pass := h.connectionRequestCreds(cpe)
	ok, err := auth.Auth(user, pass, cpe.ConnectionRequestURL)
	if err != nil {
		log.Println("Error while authenticating to CPE, err:", err)
		return err
	}
	if !ok {
		log.Println("Error while authenticating to CPE: authentication failed")
		return fmt.Errorf("connection request authentication failed")
	}

	log.Println("<-- Successfully authenticated to CPE", cpe.SerialNumber)
	return nil
}

func msgAnswer(
	callback chan []byte,
	timeMsgWasSent time.Time,
	timeOut time.Duration,
	msgAnswer []byte,
) {
	if callback == nil {
		return
	}
	if time.Since(timeMsgWasSent) > timeOut {
		log.Println("CPE took too long to answer the request, the message will be discarded")
		return
	}
	select {
	case callback <- msgAnswer:
	default:
		log.Println("CPE response callback channel full, discarding late response")
	}
}
