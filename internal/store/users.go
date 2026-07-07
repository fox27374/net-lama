package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var ErrNotFound = errors.New("not found")

type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID       string `json:"id"`
	TenantID string `json:"tenantId"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"isAdmin"`
}

const sessionLifetime = 7 * 24 * time.Hour

func (s *Store) CreateTenant(name string) (*Tenant, error) {
	t := &Tenant{ID: newID(), Name: name}
	_, err := s.db.Exec(`INSERT INTO tenants (id, name) VALUES (?, ?)`, t.ID, t.Name)
	if err != nil {
		return nil, fmt.Errorf("creating tenant: %w", err)
	}
	return t, nil
}

func (s *Store) ListTenants() ([]*Tenant, error) {
	rows, err := s.db.Query(`SELECT id, name FROM tenants ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tenants := []*Tenant{}
	for rows.Next() {
		t := &Tenant{}
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *Store) DeleteTenant(id string) error {
	_, err := s.db.Exec(`DELETE FROM tenants WHERE id = ?`, id)
	return err
}

func (s *Store) CreateUser(tenantID, username, password string, isAdmin bool) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u := &User{ID: newID(), TenantID: tenantID, Username: username, IsAdmin: isAdmin}
	var tenant any
	if tenantID != "" {
		tenant = tenantID
	}
	_, err = s.db.Exec(
		`INSERT INTO users (id, tenant_id, username, password_hash, is_admin) VALUES (?, ?, ?, ?, ?)`,
		u.ID, tenant, u.Username, string(hash), u.IsAdmin,
	)
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return u, nil
}

func (s *Store) ListUsers() ([]*User, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(tenant_id, ''), username, is_admin FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*User{}
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Username, &u.IsAdmin); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) DeleteUser(id string) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// Authenticate verifies username/password and returns the user.
func (s *Store) Authenticate(username, password string) (*User, error) {
	u := &User{}
	var hash string
	err := s.db.QueryRow(
		`SELECT id, COALESCE(tenant_id, ''), username, is_admin, password_hash FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.TenantID, &u.Username, &u.IsAdmin, &hash)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, ErrNotFound
	}
	return u, nil
}

func (s *Store) CreateSession(userID string) (string, error) {
	token := NewToken()
	_, err := s.db.Exec(
		`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, time.Now().Add(sessionLifetime),
	)
	return token, err
}

func (s *Store) SessionUser(token string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(
		`SELECT u.id, COALESCE(u.tenant_id, ''), u.username, u.is_admin
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ? AND s.expires_at > ?`,
		token, time.Now(),
	).Scan(&u.ID, &u.TenantID, &u.Username, &u.IsAdmin)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}
