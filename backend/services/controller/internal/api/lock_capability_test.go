package api

import (
	"fmt"
	"testing"

	"github.com/leandrofars/oktopus/internal/cwmp"
)

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

	result, detail := probeOntLockCapabilityCWMPWithGetter("SN-1", cwmpDataModelTR098, "tenant-a", getter)
	if result != lockProbeOK || detail != "" {
		t.Fatalf("result=%v detail=%q, want OK", result, detail)
	}
	if calls != 2 {
		t.Fatalf("getter calls=%d, want 2 roots", calls)
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

	result, _ := probeOntLockCapabilityCWMPWithGetter("SN-1", cwmpDataModelTR098, "tenant-a", getter)
	if result != lockProbeTransient {
		t.Fatalf("result=%v, want transient when one root could not be checked", result)
	}
}
