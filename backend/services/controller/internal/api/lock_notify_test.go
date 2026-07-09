package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
	"github.com/leandrofars/oktopus/internal/usp/usp_msg"
)

func TestIsLockWanIPValueChange(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{path: lockWanIPPath, want: true},
		{path: "Device.X_TELKOMSEL_OntLock.InternetWanIP", want: true},
		{path: "X_TELKOMSEL_OntLock.InternetWanIP", want: true},
		{path: "InternetWanIP", want: true},
		{path: "Device.Foo.InternetWanIP", want: false},
		{path: "Device.IP.Interface.1.InternetWanIP", want: false},
		{path: "Device.X_TELKOMSEL_OntLock.Lock", want: false},
		{path: "", want: false},
		{path: "  ", want: false},
		{path: "Device.IP.Interface.1.IPv4Address.1.IPAddress", want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			if got := isLockWanIPValueChange(tc.path); got != tc.want {
				t.Fatalf("isLockWanIPValueChange(%q)=%v want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestExtractIPFromValueChange(t *testing.T) {
	t.Parallel()

	if got := extractIPFromValueChange(nil); got != "" {
		t.Fatalf("nil → %q, want empty", got)
	}
	if got := extractIPFromValueChange(&usp_msg.Notify_ValueChange{ParamValue: " 10.1.2.3 "}); got != "10.1.2.3" {
		t.Fatalf("got %q, want 10.1.2.3", got)
	}
	if got := extractIPFromValueChange(&usp_msg.Notify_ValueChange{ParamValue: ""}); got != "" {
		t.Fatalf("empty → %q", got)
	}
}

func TestBuildLockIPValueChangeSubscriptionAdd(t *testing.T) {
	t.Parallel()

	add := buildLockIPValueChangeSubscriptionAdd()
	if !add.AllowPartial {
		t.Fatal("AllowPartial should be true")
	}
	if len(add.CreateObjs) != 1 {
		t.Fatalf("CreateObjs len=%d", len(add.CreateObjs))
	}
	obj := add.CreateObjs[0]
	if obj.ObjPath != lockSubscriptionObjPath {
		t.Fatalf("ObjPath=%q", obj.ObjPath)
	}

	got := map[string]struct {
		value    string
		required bool
	}{}
	for _, ps := range obj.ParamSettings {
		got[ps.Param] = struct {
			value    string
			required bool
		}{ps.Value, ps.Required}
	}
	want := map[string]struct {
		value    string
		required bool
	}{
		"Enable":        {"true", true},
		"NotifType":     {lockNotifTypeValueChange, true},
		"ReferenceList": {lockWanIPPath, true},
		"Persistent":    {"true", false},
	}
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Fatalf("missing param %s", k)
		}
		if g.value != w.value || g.required != w.required {
			t.Fatalf("%s: got %+v want %+v", k, g, w)
		}
	}
}

func TestLockNotifyTenantAndSN(t *testing.T) {
	t.Parallel()

	tenant, sn := lockNotifyTenantAndSN("mqtt.usp.v1.prpl-test.081074000888.api")
	if tenant != "prpl-test" || sn != "081074000888" {
		t.Fatalf("got tenant=%q sn=%q", tenant, sn)
	}
	tenant, sn = lockNotifyTenantAndSN("device.usp.v1.acme.mb_an7583.api")
	if tenant != "acme" || sn != "mb_an7583" {
		t.Fatalf("got tenant=%q sn=%q", tenant, sn)
	}
	tenant, sn = lockNotifyTenantAndSN("invalid")
	if tenant != "" || sn != "" {
		t.Fatalf("invalid subject should be empty, got %q %q", tenant, sn)
	}
}

func TestClassifyLockProbeError_SubscribeReuse(t *testing.T) {
	t.Parallel()
	// Subscribe failures still classify 7026 as unsupported for logging, but
	// ensureLockIPValueChangeSubscription must not upsert unsupported (stay on poll).
	err := fmt.Errorf("usp error %d: path does not exist in the schema", uspErrCodePathNotInSchema)
	if classifyLockProbeError(err) != lockProbeUnsupported {
		t.Fatal("7026 should be unsupported")
	}
	if classifyLockProbeError(fmt.Errorf("usp request timeout")) != lockProbeTransient {
		t.Fatal("timeout should be transient")
	}
}

func TestTouchLockNotifyOKAtPreservesExistingFields(t *testing.T) {
	store, mr := newTestRedisStateStore(t)
	defer mr.Close()
	defer store.client.Close()

	prev := lockStateStore
	lockStateStore = store
	defer func() { lockStateStore = prev }()

	ctx := context.Background()
	want := lockDeviceState{
		LastIP:      "10.1.2.3",
		LastStatus:  db.LockStatusLocked,
		LastCommand: "1",
		UpdatedAt:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.Put(ctx, "tenant-a", "SN-001", want); err != nil {
		t.Fatalf("Put: %v", err)
	}

	a := &Api{}
	a.touchLockNotifyOKAt(ctx, "tenant-a", "SN-001")

	got, found, err := store.Get(ctx, "tenant-a", "SN-001")
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	if got.LastIP != want.LastIP || got.LastStatus != want.LastStatus || got.LastCommand != want.LastCommand {
		t.Fatalf("wiped fields: %+v", got)
	}
	if got.NotifyOKAt.IsZero() {
		t.Fatal("expected NotifyOKAt set")
	}
}

func TestTouchLockNotifyOKAtCreatesOnMiss(t *testing.T) {
	store, mr := newTestRedisStateStore(t)
	defer mr.Close()
	defer store.client.Close()

	prev := lockStateStore
	lockStateStore = store
	defer func() { lockStateStore = prev }()

	ctx := context.Background()
	a := &Api{}
	a.touchLockNotifyOKAt(ctx, "tenant-a", "SN-new")

	got, found, err := store.Get(ctx, "tenant-a", "SN-new")
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	if got.LastIP != "" || got.LastStatus != "" || got.LastCommand != "" {
		t.Fatalf("expected empty IP/status/command, got %+v", got)
	}
	if got.NotifyOKAt.IsZero() {
		t.Fatal("expected NotifyOKAt set")
	}
}

func TestTouchLockNotifyOKAtSkipsWhenLockHeld(t *testing.T) {
	store, mr := newTestRedisStateStore(t)
	defer mr.Close()
	defer store.client.Close()

	prev := lockStateStore
	lockStateStore = store
	defer func() { lockStateStore = prev }()

	ctx := context.Background()
	want := lockDeviceState{
		LastIP:     "10.9.8.7",
		LastStatus: db.LockStatusUnlocked,
		UpdatedAt:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.Put(ctx, "tenant-a", "SN-001", want); err != nil {
		t.Fatalf("Put: %v", err)
	}

	unlock, ok, err := store.TryLock(ctx, "tenant-a", "SN-001", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("TryLock: ok=%v err=%v", ok, err)
	}
	defer unlock()

	a := &Api{}
	a.touchLockNotifyOKAt(ctx, "tenant-a", "SN-001")

	got, found, err := store.Get(ctx, "tenant-a", "SN-001")
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	if !got.NotifyOKAt.IsZero() {
		t.Fatalf("NotifyOKAt should remain unset while lock held, got %v", got.NotifyOKAt)
	}
	if got.LastIP != want.LastIP {
		t.Fatalf("LastIP changed: %q", got.LastIP)
	}
}
