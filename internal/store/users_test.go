package store

import "testing"

// TestSetPassword checks that the new password takes over, the old one stops
// working, and existing sessions of that user are gone.
func TestSetPassword(t *testing.T) {
	s := openTestStore(t)

	user, err := s.CreateUser("", "alice", "password123", true)
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}
	token, err := s.CreateSession(user.ID)
	if err != nil {
		t.Fatalf("creating session: %v", err)
	}

	if err := s.SetPassword(user.ID, "newpassword"); err != nil {
		t.Fatalf("setting password: %v", err)
	}

	if _, err := s.Authenticate("alice", "password123"); err != ErrNotFound {
		t.Fatalf("old password still works: %v", err)
	}
	if _, err := s.Authenticate("alice", "newpassword"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	if _, err := s.SessionUser(token); err != ErrNotFound {
		t.Fatalf("old session survived the password change: %v", err)
	}

	if err := s.SetPassword("nope", "newpassword"); err != ErrNotFound {
		t.Fatalf("unknown user: want ErrNotFound, got %v", err)
	}
}

// TestDeleteAPIKeysForUser is the other half of a reset: keys are separate
// credentials, so a reset that spared them wouldn't lock anybody out. Only
// the target's keys go.
func TestDeleteAPIKeysForUser(t *testing.T) {
	s := openTestStore(t)

	alice, err := s.CreateUser("", "alice", "password123", true)
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}
	bob, err := s.CreateUser("", "bob", "password123", true)
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}
	for _, name := range []string{"ci", "laptop"} {
		if _, err := s.CreateAPIKey(alice.ID, name); err != nil {
			t.Fatalf("creating api key: %v", err)
		}
	}
	bobKey, err := s.CreateAPIKey(bob.ID, "ci")
	if err != nil {
		t.Fatalf("creating api key: %v", err)
	}

	n, err := s.DeleteAPIKeysForUser(alice.ID)
	if err != nil {
		t.Fatalf("deleting api keys: %v", err)
	}
	if n != 2 {
		t.Fatalf("revoked %d keys, want 2", n)
	}
	keys, err := s.ListAPIKeys(alice.ID)
	if err != nil || len(keys) != 0 {
		t.Fatalf("alice keeps %d keys (err %v)", len(keys), err)
	}
	if _, err := s.APIKeyUser(bobKey.Secret); err != nil {
		t.Fatalf("bob's key died with alice's: %v", err)
	}
}

func TestUserByUsername(t *testing.T) {
	s := openTestStore(t)
	created, err := s.CreateUser("", "alice", "password123", true)
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}

	got, err := s.UserByUsername("alice")
	if err != nil {
		t.Fatalf("looking up alice: %v", err)
	}
	if got.ID != created.ID || !got.IsAdmin {
		t.Fatalf("got %+v, want id %s and isAdmin", got, created.ID)
	}
	if _, err := s.UserByUsername("nope"); err != ErrNotFound {
		t.Fatalf("unknown user: want ErrNotFound, got %v", err)
	}
}
