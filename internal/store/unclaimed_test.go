package store

import (
	"encoding/json"
	"testing"
)

// TestEnrollTokenLifecycle covers generate -> resolve tenant by token ->
// revoke -> resolve fails, and confirms a never-generated token resolves
// to nothing.
func TestEnrollTokenLifecycle(t *testing.T) {
	s := openTestStore(t)

	tenant, err := s.CreateTenant("test-tenant")
	if err != nil {
		t.Fatalf("creating tenant: %v", err)
	}

	if _, err := s.GetTenantByEnrollToken(""); err != ErrNotFound {
		t.Fatalf("empty token: err = %v, want ErrNotFound", err)
	}

	token, err := s.SetTenantEnrollToken(tenant.ID)
	if err != nil {
		t.Fatalf("generating enroll token: %v", err)
	}
	if got, err := s.GetTenantEnrollToken(tenant.ID); err != nil || got != token {
		t.Fatalf("GetTenantEnrollToken = %q, %v, want %q, nil", got, err, token)
	}

	resolved, err := s.GetTenantByEnrollToken(token)
	if err != nil {
		t.Fatalf("resolving tenant by token: %v", err)
	}
	if resolved.ID != tenant.ID {
		t.Fatalf("resolved tenant = %s, want %s", resolved.ID, tenant.ID)
	}

	// Regenerating invalidates the old token.
	newToken, err := s.SetTenantEnrollToken(tenant.ID)
	if err != nil {
		t.Fatalf("regenerating enroll token: %v", err)
	}
	if newToken == token {
		t.Fatalf("regenerated token must differ from the old one")
	}
	if _, err := s.GetTenantByEnrollToken(token); err != ErrNotFound {
		t.Fatalf("old token after regenerate: err = %v, want ErrNotFound", err)
	}

	if err := s.RevokeTenantEnrollToken(tenant.ID); err != nil {
		t.Fatalf("revoking enroll token: %v", err)
	}
	if _, err := s.GetTenantByEnrollToken(newToken); err != ErrNotFound {
		t.Fatalf("revoked token: err = %v, want ErrNotFound", err)
	}
}

// TestUpsertUnclaimedAgent covers the reconnect-updates-the-same-row
// behavior a device's backoff loop relies on, keyed by (tenant, clientId).
func TestUpsertUnclaimedAgent(t *testing.T) {
	s := openTestStore(t)

	tenantA, err := s.CreateTenant("tenant-a")
	if err != nil {
		t.Fatalf("creating tenant: %v", err)
	}
	tenantB, err := s.CreateTenant("tenant-b")
	if err != nil {
		t.Fatalf("creating tenant: %v", err)
	}

	if err := s.UpsertUnclaimedAgent(tenantA.ID, "pi-01", "v0.1.0", json.RawMessage(`["ping"]`), nil); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// Same tenant+clientId reconnecting with a newer version: same row updates.
	if err := s.UpsertUnclaimedAgent(tenantA.ID, "pi-01", "v0.2.0", json.RawMessage(`["ping","dns"]`), nil); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	// A different tenant with the same clientId is a distinct device/row.
	if err := s.UpsertUnclaimedAgent(tenantB.ID, "pi-01", "v0.1.0", nil, nil); err != nil {
		t.Fatalf("cross-tenant upsert: %v", err)
	}

	got, err := s.ListUnclaimedAgents(tenantA.ID)
	if err != nil {
		t.Fatalf("listing unclaimed agents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("tenant A: got %d unclaimed agents, want 1 (reconnect must update, not duplicate)", len(got))
	}
	if got[0].Version != "v0.2.0" {
		t.Fatalf("version = %q, want v0.2.0 (latest reported)", got[0].Version)
	}
	if string(got[0].Capabilities) != `["ping","dns"]` {
		t.Fatalf("capabilities = %s, want the latest reported list", got[0].Capabilities)
	}

	all, err := s.ListUnclaimedAgents("")
	if err != nil {
		t.Fatalf("listing all unclaimed agents: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d unclaimed agents across tenants, want 2", len(all))
	}

	if err := s.DeleteUnclaimedAgent(got[0].ID); err != nil {
		t.Fatalf("deleting unclaimed agent: %v", err)
	}
	if got, err := s.ListUnclaimedAgents(tenantA.ID); err != nil || len(got) != 0 {
		t.Fatalf("after delete: got %d, %v, want 0, nil", len(got), err)
	}
}
