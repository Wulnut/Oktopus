package handler

import (
	"log"
	"time"
)

type offlineCPE struct {
	sn     string
	tenant string
}

func (h *Handler) HandleCpeStatus() {
	for {
		h.handleCpeStatusOnce()
		time.Sleep(10 * time.Second)
	}
}

func (h *Handler) handleCpeStatusOnce() {
	var offline []offlineCPE

	h.mu.Lock()
	for sn, cpe := range h.Cpes {
		if sn == "" {
			continue
		}
		if h.acsConfig.DebugMode {
			log.Println("Checking CPE " + sn + " status")
		}
		if time.Since(cpe.LastConnection) > h.acsConfig.KeepAliveInterval {
			log.Printf("LastConnection: %s, KeepAliveInterval: %s", cpe.LastConnection, h.acsConfig.KeepAliveInterval)
			log.Println("CPE", sn, "is offline")
			offline = append(offline, offlineCPE{sn: sn, tenant: cpeTenantSlug(cpe)})
		}
	}
	h.mu.Unlock()

	// Process all timed-out CPEs in the same cycle, but re-check before deleting
	// because an Inform can arrive between the scan and this cleanup step.
	for _, item := range offline {
		h.mu.Lock()
		cpe, ok := h.Cpes[item.sn]
		if ok && time.Since(cpe.LastConnection) > h.acsConfig.KeepAliveInterval {
			delete(h.Cpes, item.sn)
			h.mu.Unlock()
			h.pub(cwmpStatusSubject(cpeTenantSlug(cpe), item.sn), []byte("0"))
			continue
		}
		h.mu.Unlock()
	}
}
