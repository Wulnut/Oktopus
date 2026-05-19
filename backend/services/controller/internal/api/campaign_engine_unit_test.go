package api

import (
	"testing"

	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/entity"
)

func TestDeviceMatchesCampaignHardware_CaseInsensitive(t *testing.T) {
	c := db.Campaign{Vendor: "Huawei", Model: "HG8145", HWVersion: "rev-A"}
	d := entity.Device{Vendor: "huawei", Model: "hg8145", HWVersion: "REV-A"}
	if !deviceMatchesCampaignHardware(d, c) {
		t.Fatal("expected case-insensitive hardware match")
	}
}

func TestDeviceMatchesCampaignHardware_TrimSpace(t *testing.T) {
	c := db.Campaign{Vendor: " Vendor ", Model: "Model", HWVersion: "hw"}
	d := entity.Device{Vendor: "Vendor", Model: " Model ", HWVersion: "hw"}
	if !deviceMatchesCampaignHardware(d, c) {
		t.Fatal("expected match after trimming whitespace")
	}
}

func TestDeviceMatchesCampaignHardware_NoMatch(t *testing.T) {
	c := db.Campaign{Vendor: "A", Model: "M", HWVersion: "1"}
	d := entity.Device{Vendor: "A", Model: "M", HWVersion: "2"}
	if deviceMatchesCampaignHardware(d, c) {
		t.Fatal("expected no match for different hw version")
	}
}
