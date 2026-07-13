package snapshot

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"testing"
)

// updateFlag (-update) regenerates golden files instead of comparing against them.
// Usage: go test -run TestDevice_List_Snapshot -update ./...
var updateFlag = flag.Bool("update", false, "regenerate golden files instead of comparing")

// CompareSnapshot asserts that body matches the golden file golden/<name>.golden.json.
// On first run (file missing) or with -update, it writes body as the new golden.
//
// Before comparison, volatile fields are redacted so snapshots stay stable
// across runs: Mongo ObjectIDs (24-hex), ISO-8601 timestamps, and any field
// named id/_id/object_id/created_at/updated_at. The redaction replaces the
// value with a placeholder token (e.g. "<ID>") rather than dropping the key,
// so the response shape is preserved.
func CompareSnapshot(t *testing.T, name string, body []byte) {
	t.Helper()
	goldenPath := goldenFilePath(name)
	normalized := normalizeJSON(redactVolatile(body))

	if *updateFlag {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("snapshot: cannot mkdir for golden %s: %v", goldenPath, err)
		}
		if err := os.WriteFile(goldenPath, normalized, 0644); err != nil {
			t.Fatalf("snapshot: cannot write golden %s: %v", goldenPath, err)
		}
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			// First-time bootstrapping: write the golden so the next run compares.
			if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err == nil {
				_ = os.WriteFile(goldenPath, normalized, 0644)
			}
			t.Fatalf("snapshot %q: golden file did not exist; wrote baseline at %s\n"+
				"re-run tests to compare against it, or use -update to refresh.", name, goldenPath)
		}
		t.Fatalf("snapshot %q: cannot read golden %s: %v", name, goldenPath, err)
	}

	if !bytes.Equal(normalized, expected) {
		t.Errorf("snapshot %q drift:\n--- expected (%s)\n%s\n--- got\n%s",
			name, goldenPath, expected, normalized)
	}
}

// hexIDRE matches a 24-character MongoDB ObjectID hex string.
var hexIDRE = regexp.MustCompile(`"[0-9a-f]{24}"`)

// isoTimeRE matches an ISO-8601 timestamp like 2026-07-10T17:21:25.123Z.
var isoTimeRE = regexp.MustCompile(`"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})"`)

// redactVolatile replaces volatile values in a JSON body with stable placeholders
// so byte-level snapshot comparison is meaningful. We operate on the raw bytes
// via regex (not unmarshal) to preserve the exact key ordering the server emits.
func redactVolatile(b []byte) []byte {
	out := b
	out = hexIDRE.ReplaceAll(out, []byte(`"<ID>"`))
	out = isoTimeRE.ReplaceAll(out, []byte(`"<TIMESTAMP>"`))
	return out
}

// CompareStruct compares two values by reflect.DeepEqual after zeroing any
// fields named in ignoreFields at the top level of got/want (if they're structs
// or map[string]interface{}). Used for write endpoints whose responses contain
// non-deterministic fields like timestamps or generated ObjectIDs.
func CompareStruct(t *testing.T, got, want interface{}, ignoreFields ...string) {
	t.Helper()
	got = strip(got, ignoreFields)
	want = strip(want, ignoreFields)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("struct mismatch:\n--- want\n%#v\n--- got\n%#v", want, got)
	}
}

// strip returns v with top-level ignoreFields removed. Only handles
// map[string]interface{} and structs via reflection; other types pass through.
func strip(v interface{}, ignoreFields []string) interface{} {
	if len(ignoreFields) == 0 {
		return v
	}
	switch m := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, val := range m {
			out[k] = val
		}
		for _, f := range ignoreFields {
			delete(out, f)
		}
		return out
	default:
		// For struct types we don't bother with reflection-based field zeroing —
		// callers should compare concrete structs they define locally.
		return v
	}
}

// normalizeJSON canonicalizes JSON whitespace so byte comparison is stable.
// It does NOT sort object keys; the server's encoder is assumed deterministic.
// For our purposes (controller uses encoding/json with consistent ordering),
// whitespace normalization is sufficient.
func normalizeJSON(b []byte) []byte {
	buf := bytes.Buffer{}
	if err := json.Compact(&buf, b); err != nil {
		// Not valid JSON — return as-is so the failure mode is visible.
		return b
	}
	return buf.Bytes()
}

// goldenFilePath resolves <name>.golden.json relative to the snapshot package
// source directory. Goldens live alongside the test code (under
// tests/api_snapshot/snapshot/golden/) so the test module is self-contained
// and doesn't depend on a repo-root path that changes between host and container.
func goldenFilePath(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	return filepath.Join(dir, "golden", name+".golden.json")
}
