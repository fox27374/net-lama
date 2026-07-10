package store

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestAPIKeyLifecycle exercises create -> lookup by secret -> revoke ->
// lookup fails, and checks that listing never leaks the secret or hash.
func TestAPIKeyLifecycle(t *testing.T) {
	s := openTestStore(t)

	user, err := s.CreateUser("", "alice", "password123", true)
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}

	key, err := s.CreateAPIKey(user.ID, "ci")
	if err != nil {
		t.Fatalf("creating api key: %v", err)
	}
	if key.Secret == "" {
		t.Fatal("expected a secret on creation")
	}
	if len(key.Prefix) != apiKeyPrefixLen || key.Prefix != key.Secret[:apiKeyPrefixLen] {
		t.Fatalf("prefix %q does not match secret %q", key.Prefix, key.Secret)
	}

	// Lookup by the presented secret succeeds and resolves the owning user.
	got, err := s.APIKeyUser(key.Secret)
	if err != nil {
		t.Fatalf("looking up by secret: %v", err)
	}
	if got.ID != user.ID {
		t.Fatalf("resolved user %q, want %q", got.ID, user.ID)
	}

	// A garbage secret never resolves.
	if _, err := s.APIKeyUser("nlk_not-a-real-key"); err != ErrNotFound {
		t.Fatalf("garbage secret: err = %v, want ErrNotFound", err)
	}

	// Listing never includes the secret or hash.
	list, err := s.ListAPIKeys(user.ID)
	if err != nil {
		t.Fatalf("listing keys: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 key, got %d", len(list))
	}
	if list[0].Secret != "" {
		t.Fatal("ListAPIKeys leaked the secret")
	}
	if list[0].Prefix != key.Prefix {
		t.Fatalf("listed prefix %q, want %q", list[0].Prefix, key.Prefix)
	}

	// Revoking someone else's key is rejected.
	other, err := s.CreateUser("", "bob", "password123", true)
	if err != nil {
		t.Fatalf("creating second user: %v", err)
	}
	if err := s.DeleteAPIKey(key.ID, other.ID); err != ErrNotFound {
		t.Fatalf("deleting another user's key: err = %v, want ErrNotFound", err)
	}

	// Revoking your own key succeeds, and the secret no longer resolves.
	if err := s.DeleteAPIKey(key.ID, user.ID); err != nil {
		t.Fatalf("revoking key: %v", err)
	}
	if _, err := s.APIKeyUser(key.Secret); err != ErrNotFound {
		t.Fatalf("lookup after revoke: err = %v, want ErrNotFound", err)
	}
}

// TestAPIKeyDeletedWithUser verifies deleting a user cascades to their
// API keys (the same ON DELETE CASCADE pattern used for sessions).
func TestAPIKeyDeletedWithUser(t *testing.T) {
	s := openTestStore(t)

	user, err := s.CreateUser("", "carol", "password123", true)
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}
	key, err := s.CreateAPIKey(user.ID, "laptop")
	if err != nil {
		t.Fatalf("creating api key: %v", err)
	}

	if err := s.DeleteUser(user.ID); err != nil {
		t.Fatalf("deleting user: %v", err)
	}

	if _, err := s.APIKeyUser(key.Secret); err != ErrNotFound {
		t.Fatalf("lookup after user delete: err = %v, want ErrNotFound", err)
	}
}
