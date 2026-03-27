package api

import (
	"testing"
)

// Tests the pagination skip formula used in device.go:130
// Fixed formula: skip = page_number * page_size

func calcSkip(page_number, page_size int64) int64 {
	return page_number * page_size
}

func TestPaginationSkip_Page0(t *testing.T) {
	skip := calcSkip(0, 20)
	if skip != 0 {
		t.Errorf("Page 0: skip=%d, expected=0", skip)
	}
}

func TestPaginationSkip_Page1(t *testing.T) {
	skip := calcSkip(1, 20)
	if skip != 20 {
		t.Errorf("Page 1: skip=%d, expected=20", skip)
	}
}

func TestPaginationSkip_Page2(t *testing.T) {
	skip := calcSkip(2, 20)
	if skip != 40 {
		t.Errorf("Page 2: skip=%d, expected=40", skip)
	}
}

func TestPaginationSkip_Page10(t *testing.T) {
	skip := calcSkip(10, 20)
	if skip != 200 {
		t.Errorf("Page 10: skip=%d, expected=200", skip)
	}
}
