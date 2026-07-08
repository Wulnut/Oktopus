package cwmp_handler

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"

	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/cwmp"
	"github.com/OktopUSP/oktopus/backend/services/mtp/adapter/internal/db"
)

func (h *Handler) HandleDeviceInfo(device, tenantSlug string, data []byte, ack func()) {
	defer ack()
	if tenantSlug == "" {
		log.Printf("WARNING: empty tenant for CWMP device %s, skipping info message", device)
		return
	}
	log.Printf("Device %s info, tenant: %s", device, tenantSlug)

	deviceInfo, err := parseDeviceInfoMsg(data)
	if err != nil {
		log.Printf("REJECTED: CWMP device %s: %v", device, err)
		return
	}
	if deviceInfo.SN == "" {
		log.Printf("WARNING: empty SN for CWMP device %s, skipping info message", device)
		return
	}

	deviceInfo.TenantID = tenantSlug
	if deviceExists, _ := h.db.DeviceExists(deviceInfo.SN); !deviceExists {
		fmtDeviceInfo, _ := json.Marshal(deviceInfo)
		h.nc.Publish("device.v1."+tenantSlug+".new", fmtDeviceInfo)
	}
	err = h.db.CreateDevice(deviceInfo)
	if err != nil {
		log.Printf("Failed to create device: %v", err)
		return
	}
	onlineData, _ := json.Marshal(deviceInfo)
	h.nc.Publish("device.v1."+tenantSlug+".online", onlineData)
}

func parseDeviceInfoMsg(data []byte) (db.Device, error) {
	var inform cwmp.CWMPInform
	if err := xml.Unmarshal(data, &inform); err != nil {
		return db.Device{}, fmt.Errorf("invalid CWMP Inform XML: %w", err)
	}
	if inform.DeviceId.SerialNumber == "" {
		return db.Device{}, fmt.Errorf("missing serial number in Inform")
	}

	var device db.Device
	device.Vendor = inform.DeviceId.Manufacturer
	device.Model = ""
	device.Version = inform.GetSoftwareVersion()
	device.ProductClass = inform.DeviceId.ProductClass
	device.SN = inform.DeviceId.SerialNumber
	device.Cwmp = db.Online
	device.Status = db.Online
	device.DataModel = inform.GetDataModelType()

	return device, nil
}
