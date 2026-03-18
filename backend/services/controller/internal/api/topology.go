package api

import (
	"net/http"

	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
	"github.com/leandrofars/oktopus/internal/usp/usp_utils"
)

// GET /api/device/{sn}/{mtp}/topology
// Returns associated WiFi clients and all hosts
func (a *Api) deviceTopology(w http.ResponseWriter, r *http.Request) {
	sn := getSerialNumberFromRequest(r)
	mtp, err := getMtpFromRequest(r, w)
	if err != nil {
		return
	}
	if mtp == "" {
		var ok bool
		mtp, ok = deviceStateOK(w, a.nc, sn)
		if !ok {
			return
		}
	}
	msg := usp_utils.NewGetMsg(usp_msg.Get{
		ParamPaths: []string{
			"Device.WiFi.AccessPoint.",
			"Device.Hosts.Host.",
		},
		MaxDepth: 3,
	})
	sendUspMsg(msg, sn, w, a.nc, mtp)
}
