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
