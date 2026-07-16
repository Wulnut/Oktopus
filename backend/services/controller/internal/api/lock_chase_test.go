package api

import (
	"testing"

	"github.com/leandrofars/oktopus/internal/db"
)

func TestShouldChaseLockOnStartup_ActiveTenantAlwaysReconciles(t *testing.T) {
	tenant := db.Tenant{Slug: "tenant-a", Status: db.TenantStatusActive}
	if !shouldChaseLockOnStartup(tenant) {
		t.Fatal("active tenant must be reconciled even when ONT Lock master is disabled so stale locks are removed")
	}
}

func TestShouldChaseLockOnStartup_SkipsInvalidTenants(t *testing.T) {
	for _, tenant := range []db.Tenant{
		{Slug: "", Status: db.TenantStatusActive},
		{Slug: "tenant-a", Status: db.TenantStatusDisabled},
	} {
		if shouldChaseLockOnStartup(tenant) {
			t.Fatalf("tenant %#v should not be chased", tenant)
		}
	}
}
