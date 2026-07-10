package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// apiKeyPrefixLen is how many leading characters of a generated secret
// (including the "nlk_" marker) are stored/displayed for identification.
const apiKeyPrefixLen = 12

// APIKey is the API representation of a stored key. Secret is only ever
// populated by CreateAPIKey — it is never stored, logged or returned again.
type APIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	Secret     string     `json:"secret,omitempty"`
}

func hashAPIKeySecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// CreateAPIKey generates a new API key secret for userID, stores its
// SHA-256 hash and a display prefix, and returns the key with Secret
// populated — the one and only time the full secret is available.
func (s *Store) CreateAPIKey(userID, name string) (*APIKey, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generating api key: %w", err)
	}
	secret := "nlk_" + hex.EncodeToString(raw)

	k := &APIKey{
		ID:        newID(),
		UserID:    userID,
		Name:      name,
		Prefix:    secret[:apiKeyPrefixLen],
		CreatedAt: time.Now(),
		Secret:    secret,
	}
	_, err := s.db.Exec(
		`INSERT INTO api_keys (id, user_id, name, key_hash, prefix, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		k.ID, k.UserID, k.Name, hashAPIKeySecret(secret), k.Prefix, k.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating api key: %w", err)
	}
	return k, nil
}

// ListAPIKeys returns userID's keys, newest first. Never includes the
// hash or the secret.
func (s *Store) ListAPIKeys(userID string) ([]*APIKey, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, name, prefix, created_at, last_used_at
		 FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []*APIKey{}
	for rows.Next() {
		k := &APIKey{}
		var lastUsed sql.NullTime
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.CreatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			t := lastUsed.Time
			k.LastUsedAt = &t
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// DeleteAPIKey revokes a key, scoped to the owning user so a user can only
// ever revoke their own keys. Returns ErrNotFound if id doesn't belong to
// userID (or doesn't exist).
func (s *Store) DeleteAPIKey(id, userID string) error {
	res, err := s.db.Exec(`DELETE FROM api_keys WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// APIKeyUser hashes the presented secret, looks up the owning user and
// updates last_used_at on a hit. The secret itself is never logged.
func (s *Store) APIKeyUser(secret string) (*User, error) {
	hash := hashAPIKeySecret(secret)
	u := &User{}
	var keyID string
	err := s.db.QueryRow(
		`SELECT k.id, u.id, COALESCE(u.tenant_id, ''), u.username, u.is_admin
		 FROM api_keys k JOIN users u ON u.id = k.user_id
		 WHERE k.key_hash = ?`,
		hash,
	).Scan(&keyID, &u.ID, &u.TenantID, &u.Username, &u.IsAdmin)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Best-effort bookkeeping; a failure here must not fail auth.
	_, _ = s.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, time.Now(), keyID)
	return u, nil
}
