package api

import (
	"fmt"
	"testing"

	"github.com/leandrofars/oktopus/internal/cwmp"
	"github.com/leandrofars/oktopus/internal/db"
)

func TestNormalizeActualLockValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		raw    string
		want   db.DeviceLockStatus
		wantOK bool
	}{
		{name: "zero", raw: "0", want: db.LockStatusUnlocked, wantOK: true},
		{name: "false", raw: "false", want: db.LockStatusUnlocked, wantOK: true},
		{name: "unlocked", raw: "UNLOCKED", want: db.LockStatusUnlocked, wantOK: true},
		{name: "one", raw: "1", want: db.LockStatusLocked, wantOK: true},
		{name: "true", raw: " TRUE ", want: db.LockStatusLocked, wantOK: true},
		{name: "locked", raw: "locked", want: db.LockStatusLocked, wantOK: true},
		{name: "empty", raw: "", wantOK: false},
		{name: "unknown", raw: "enabled", wantOK: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := normalizeActualLockValue(tc.raw)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("normalizeActualLockValue(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestClassifyLockProbeError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want lockProbeResult
	}{
		{name: "nil", err: nil, want: lockProbeOK},
		{
			name: "usp 7026",
			err:  fmt.Errorf("usp error %d: CheckPathProperties: Path (Device.X_TELKOMSEL_OntLock) does not exist in the schema", uspErrCodePathNotInSchema),
			want: lockProbeUnsupported,
		},
		{
			name: "schema missing text",
			err:  fmt.Errorf("path Device.X_TELKOMSEL_OntLock.Lock does not exist in the schema"),
			want: lockProbeUnsupported,
		},
		{
			name: "parameter not in USP response",
			err:  fmt.Errorf("parameter %s not found in USP response", lockParameterPath),
			want: lockProbeUnsupported,
		},
		{
			name: "timeout",
			err:  fmt.Errorf("usp request timeout"),
			want: lockProbeTransient,
		},
		{
			name: "transport failure",
			err:  fmt.Errorf("usp request failed with status 502: bad gateway"),
			want: lockProbeTransient,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyLockProbeError(tc.err)
			if got != tc.want {
				t.Fatalf("classifyLockProbeError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestShouldProbeOntLockCapability(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		trigger   string
		hasRow    bool
		optOut    bool
		wantProbe bool
	}{
		{name: "optOut", trigger: lockTriggerOnline, hasRow: true, optOut: true, wantProbe: false},
		{name: "optOut poll", trigger: lockTriggerIPChangePoll, hasRow: false, optOut: true, wantProbe: false},
		{name: "unsupported+poll", trigger: lockTriggerIPChangePoll, hasRow: true, optOut: false, wantProbe: false},
		{name: "unsupported+notify", trigger: lockTriggerIPChangeNotify, hasRow: true, optOut: false, wantProbe: false},
		{name: "unsupported+chase", trigger: lockTriggerChase, hasRow: true, optOut: false, wantProbe: false},
		{name: "unsupported+online", trigger: lockTriggerOnline, hasRow: true, optOut: false, wantProbe: true},
		{name: "no row+poll", trigger: lockTriggerIPChangePoll, hasRow: false, optOut: false, wantProbe: true},
		{name: "no row+online", trigger: lockTriggerOnline, hasRow: false, optOut: false, wantProbe: true},
		{name: "no row+chase", trigger: lockTriggerChase, hasRow: false, optOut: false, wantProbe: true},
		{name: "no row+notify", trigger: lockTriggerIPChangeNotify, hasRow: false, optOut: false, wantProbe: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldProbeOntLockCapability(tc.trigger, tc.hasRow, tc.optOut)
			if got != tc.wantProbe {
				t.Fatalf("shouldProbeOntLockCapability(%q, hasRow=%v, optOut=%v) = %v, want %v",
					tc.trigger, tc.hasRow, tc.optOut, got, tc.wantProbe)
			}
		})
	}
}

func TestProbeOntLockCapabilityCWMPTriesAlternateRootAfterPartialFirstRoot(t *testing.T) {
	calls := 0
	getter := func(_ string, names []string, _ string) (cwmp.GetParameterValuesResponse, error) {
		calls++
		if calls == 1 {
			return cwmp.GetParameterValuesResponse{ParameterList: []cwmp.ParameterValueStruct{
				{Name: names[0], Value: "0"},
			}}, nil
		}
		return cwmp.GetParameterValuesResponse{ParameterList: []cwmp.ParameterValueStruct{
			{Name: names[0], Value: "0"},
			{Name: names[1], Value: "203.0.113.10"},
		}}, nil
	}

	got := probeOntLockCapabilityCWMPWithGetter("SN-1", cwmpDataModelTR098, "tenant-a", getter)
	if got.Result != lockProbeOK || got.Detail != "" {
		t.Fatalf("snapshot=%+v, want OK", got)
	}
	if got.LockStatus != db.LockStatusUnlocked || got.ReportedIP != "203.0.113.10" || got.CWMPRoot != lockCWMPRootPrefix(cwmpDataModelTR181) {
		t.Fatalf("unexpected snapshot values: %+v", got)
	}
	if calls != 2 {
		t.Fatalf("getter calls=%d, want 2 roots", calls)
	}
}

func TestProbeOntLockCapabilityUSPRetainsValues(t *testing.T) {
	getter := func(_ string, path, _, _ string) (string, error) {
		switch path {
		case lockParameterPath:
			return "true", nil
		case lockWanIPPath:
			return "203.0.113.20", nil
		default:
			return "", fmt.Errorf("unexpected path %s", path)
		}
	}

	got := probeOntLockCapabilityUSPWithGetter("SN-1", "mqtt", "tenant-a", getter)
	if got.Result != lockProbeOK || got.LockStatus != db.LockStatusLocked || got.ReportedIP != "203.0.113.20" {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
}

func TestProbeOntLockCapabilityUSPMalformedLockIsTransient(t *testing.T) {
	getter := func(_ string, path, _, _ string) (string, error) {
		if path == lockParameterPath {
			return "enabled", nil
		}
		return "203.0.113.20", nil
	}

	got := probeOntLockCapabilityUSPWithGetter("SN-1", "mqtt", "tenant-a", getter)
	if got.Result != lockProbeTransient {
		t.Fatalf("snapshot=%+v, want transient", got)
	}
}

func TestProbeOntLockCapabilityCWMPMalformedLockIsTransient(t *testing.T) {
	getter := func(_ string, names []string, _ string) (cwmp.GetParameterValuesResponse, error) {
		return cwmp.GetParameterValuesResponse{ParameterList: []cwmp.ParameterValueStruct{
			{Name: names[0], Value: "enabled"},
			{Name: names[1], Value: "203.0.113.10"},
		}}, nil
	}

	got := probeOntLockCapabilityCWMPWithGetter("SN-1", cwmpDataModelTR098, "tenant-a", getter)
	if got.Result != lockProbeTransient {
		t.Fatalf("snapshot=%+v, want transient", got)
	}
}

func TestProbeOntLockCapabilityCWMPDoesNotMarkUnsupportedWhenEitherRootIsTransient(t *testing.T) {
	calls := 0
	getter := func(_ string, _ []string, _ string) (cwmp.GetParameterValuesResponse, error) {
		calls++
		if calls == 1 {
			return cwmp.GetParameterValuesResponse{}, fmt.Errorf("cwmp request timeout")
		}
		return cwmp.GetParameterValuesResponse{}, fmt.Errorf("path does not exist in the schema")
	}

	got := probeOntLockCapabilityCWMPWithGetter("SN-1", cwmpDataModelTR098, "tenant-a", getter)
	if got.Result != lockProbeTransient {
		t.Fatalf("snapshot=%+v, want transient when one root could not be checked", got)
	}
}
