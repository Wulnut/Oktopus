// Package load hosts long-running, resource-intensive stress tests that are
// NOT part of the default `go test ./...` run. They are gated behind the
// RUN_LOAD=1 environment variable so CI can opt in on a schedule without
// slowing down the standard test-snapshot job.
//
// Important: the snapshot environment has no adapter subscribed to NATS, so
// routes that go through NATS (GET /device, /info/*, /device/{sn}/get, ...)
// will all time out. Load tests here therefore target pure-Mongo routes
// only: /firmware, /lock/audit, /lock/policies, /scripts, /campaigns.
//
// Run with:
//
//	RUN_LOAD=1 go test -v -count=1 -timeout=600s -run TestLoad ./load/...
package load

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// requireLoadGate skips the entire package unless RUN_LOAD=1 is set.
func requireLoadGate(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_LOAD") != "1" {
		t.Skip("load tests skipped; set RUN_LOAD=1 to enable")
	}
}

// TestLoad_FirmwareList_100Concurrent fires 100 goroutines that each hit
// GET /firmware (pure Mongo) in parallel and asserts every request returns
// 200 with a JSON body, plus reports aggregate throughput.
func TestLoad_FirmwareList_100Concurrent(t *testing.T) {
	requireLoadGate(t)
	env := snapshot.Setup(t, []string{"firmware.json"})
	client := snapshot.NewClient(env.JWT)

	const concurrency = 100
	var wg sync.WaitGroup
	wg.Add(concurrency)
	var (
		failures   int64
		totalLatNS int64
	)
	start := time.Now()

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			reqStart := time.Now()
			status, body, err := client.Get(env.TenantURL("/firmware"))
			atomic.AddInt64(&totalLatNS, time.Since(reqStart).Nanoseconds())
			if err != nil {
				atomic.AddInt64(&failures, 1)
				return
			}
			if status != 200 || len(body) == 0 {
				atomic.AddInt64(&failures, 1)
				return
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	if failures > 0 {
		t.Fatalf("%d/%d concurrent requests failed", failures, concurrency)
	}
	avgMS := float64(atomic.LoadInt64(&totalLatNS)) / float64(concurrency) / 1e6
	t.Logf("100 concurrent GET /firmware: total=%s avg=%2.2fms", elapsed, avgMS)
}

// TestLoad_FirmwareList_RampUp steps up concurrency (1, 10, 50, 100) and
// records per-stage latency samples. Asserts P99 stays under a generous
// ceiling (500ms here — the in-memory httptest server should be fast).
func TestLoad_FirmwareList_RampUp(t *testing.T) {
	requireLoadGate(t)
	env := snapshot.Setup(t, []string{"firmware.json"})
	client := snapshot.NewClient(env.JWT)

	stages := []int{1, 10, 50, 100}
	const p99CeilingMS = 500.0

	for _, n := range stages {
		samples := rampStage(t, client, env, "/firmware", n)
		p50 := percentileMS(samples, 50)
		p99 := percentileMS(samples, 99)
		t.Logf("stage n=%d: p50=%2.2fms p99=%2.2fms", n, p50, p99)
		if p99 > p99CeilingMS {
			t.Errorf("stage n=%d: P99 latency %2.2fms exceeds ceiling %2.2fms",
				n, p99, p99CeilingMS)
		}
	}
}

// rampStage hits `path` with `n` concurrent requests, returns per-request
// latency in nanoseconds.
func rampStage(t *testing.T, client *snapshot.Client, env *snapshot.Env, path string, n int) []int64 {
	t.Helper()
	var (
		wg        sync.WaitGroup
		samplesMu sync.Mutex
		samples   = make([]int64, 0, n)
		failures  int64
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			s := time.Now()
			status, body, err := client.Get(env.TenantURL(path))
			lat := time.Since(s).Nanoseconds()
			samplesMu.Lock()
			samples = append(samples, lat)
			samplesMu.Unlock()
			if err != nil || status != 200 || len(body) == 0 {
				atomic.AddInt64(&failures, 1)
			}
		}()
	}
	wg.Wait()
	if failures > 0 {
		t.Fatalf("stage n=%d: %d failures", n, failures)
	}
	return samples
}

// percentileMS returns the p-th percentile latency in milliseconds.
func percentileMS(samplesNS []int64, p float64) float64 {
	if len(samplesNS) == 0 {
		return 0
	}
	sorted := make([]int64, len(samplesNS))
	copy(sorted, samplesNS)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)-1) * p / 100)
	return float64(sorted[idx]) / 1e6
}

// TestLoad_LockAudit_LargeResultSet seeds 10k lock audit logs and walks
// the /lock/audit route across pages (page_size capped at 100 by the API),
// asserting every page returns 200 and the cumulative item count matches
// the seed count.
func TestLoad_LockAudit_LargeResultSet(t *testing.T) {
	requireLoadGate(t)
	env := snapshot.Setup(t, nil)
	client := snapshot.NewClient(env.JWT)

	const seedCount = 10000
	const pageSize = 100 // API max
	seedLockAuditLogs(t, env, seedCount)

	var totalSeen int64
	pages := (seedCount + pageSize - 1) / pageSize
	for page := 0; page < pages; page++ {
		url := fmt.Sprintf("%s?page_number=%d&page_size=%d",
			env.TenantURL("/lock/audit"), page, pageSize)
		status, body, err := client.Get(url)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if status != 200 {
			t.Fatalf("page %d: want 200 got %d, body=%s", page, status, body)
		}
		var resp struct {
			Items []map[string]interface{} `json:"items"`
			Total int64                    `json:"total"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("page %d: bad JSON: %v", page, err)
		}
		totalSeen += int64(len(resp.Items))
		if resp.Total != seedCount {
			t.Fatalf("page %d: total=%d want %d", page, resp.Total, seedCount)
		}
	}
	if totalSeen != seedCount {
		t.Errorf("walked %d items across pages, want %d", totalSeen, seedCount)
	}
	t.Logf("10k audit logs walked across %d pages: %d items", pages, totalSeen)
}

// seedLockAuditLogs inserts n synthetic lock audit log rows into the
// snapshot tenant's lock_audit_logs collection.
func seedLockAuditLogs(t *testing.T, env *snapshot.Env, n int) {
	t.Helper()
	coll := env.Mongo.Database(fmt.Sprintf("tenant_%s_general", snapshot.SnapshotTenantSlug)).
		Collection("lock_audit_logs")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := coll.DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("clear audit logs: %v", err)
	}
	const batchSize = 1000
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	base := time.Now().Add(-24 * time.Hour)
	for i := 0; i < n; i += batchSize {
		end := i + batchSize
		if end > n {
			end = n
		}
		docs := make([]interface{}, 0, end-i)
		for j := i; j < end; j++ {
			docs = append(docs, bson.M{
				"sn":          fmt.Sprintf("SN-LOAD-%05d", j),
				"action":      []string{"lock", "unlock", "chase"}[rng.Intn(3)],
				"policy_type": "whitelist",
				"status":      "locked",
				"created_at":  base.Add(time.Duration(j) * time.Second),
			})
		}
		if _, err := coll.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil {
			t.Fatalf("seed batch %d: %v", i/batchSize, err)
		}
	}
}

// TestLoad_MixedReadRoutes_HotLoop hammers a rotation of pure-Mongo read
// routes (/firmware, /scripts, /campaigns, /lock/audit, /lock/policies)
// for `durationSec` seconds across `workers` goroutines and reports rps.
// This substitutes for the plan's /info/* hot loop, which needs NATS.
func TestLoad_MixedReadRoutes_HotLoop(t *testing.T) {
	requireLoadGate(t)
	env := snapshot.Setup(t, []string{
		"firmware.json",
		"scripts.json",
		"campaigns.json",
		"lock_policies.json",
	})
	client := snapshot.NewClient(env.JWT)

	durationSec := 10
	if v := os.Getenv("LOAD_INFO_DURATION_SEC"); v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil && d > 0 {
			durationSec = int(d.Seconds())
		}
	}
	const workers = 20
	deadline := time.Now().Add(time.Duration(durationSec) * time.Second)
	routes := []string{
		"/firmware",
		"/scripts",
		"/campaigns",
		"/lock/audit?page_number=0&page_size=25",
		"/lock/policies",
	}

	var (
		wg       sync.WaitGroup
		requests int64
		failures int64
	)
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			i := 0
			for time.Now().Before(deadline) {
				route := routes[i%len(routes)]
				i++
				status, body, err := client.Get(env.TenantURL(route))
				atomic.AddInt64(&requests, 1)
				if err != nil || status != 200 || len(body) == 0 {
					atomic.AddInt64(&failures, 1)
					continue
				}
			}
		}(w)
	}
	wg.Wait()

	if failures > 0 {
		t.Errorf("%d/%d requests failed", failures, atomic.LoadInt64(&requests))
	}
	t.Logf("mixed read routes hot loop: workers=%d duration=%ds reqs=%d rps=%2.1f",
		workers, durationSec, atomic.LoadInt64(&requests),
		float64(atomic.LoadInt64(&requests))/float64(durationSec))
}
