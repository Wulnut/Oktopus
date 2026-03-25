package api

import (
	"testing"
)

// Tests the pagination skip formula used in device.go:130
// Current code: skip = page_number * (page_size - 1)
// Correct:      skip = page_number * page_size

func TestPaginationSkip_Page0(t *testing.T) {
	var page_number int64 = 0
	var page_size int64 = 20

	skip := page_number * (page_size - 1) // current formula
	expected := page_number * page_size    // correct formula

	// Page 0: both give 0, so this passes even with the bug
	if skip != expected {
		t.Errorf("Page 0: skip=%d, expected=%d", skip, expected)
	}
}

func TestPaginationSkip_Page1(t *testing.T) {
	var page_number int64 = 1
	var page_size int64 = 20

	skip := page_number * (page_size - 1)
	expected := page_number * page_size

	if skip != expected {
		t.Errorf("Page 1: current formula gives skip=%d, correct is skip=%d (off by %d)", skip, expected, expected-skip)
	}
}

func TestPaginationSkip_Page2(t *testing.T) {
	var page_number int64 = 2
	var page_size int64 = 20

	skip := page_number * (page_size - 1)
	expected := page_number * page_size

	if skip != expected {
		t.Errorf("Page 2: current formula gives skip=%d, correct is skip=%d (off by %d)", skip, expected, expected-skip)
	}
}

func TestPaginationSkip_Page10(t *testing.T) {
	var page_number int64 = 10
	var page_size int64 = 20

	skip := page_number * (page_size - 1)
	expected := page_number * page_size

	if skip != expected {
		t.Errorf("Page 10: current formula gives skip=%d, correct is skip=%d (off by %d)", skip, expected, expected-skip)
	}
}
