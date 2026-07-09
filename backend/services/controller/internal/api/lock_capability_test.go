package api

import (
	"fmt"
	"testing"
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
