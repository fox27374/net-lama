package store

// Tenanted is implemented by every row that belongs to exactly one tenant.
// It exists so callers can enforce tenant scoping generically instead of
// reaching for a TenantID field on each concrete type.
type Tenanted interface {
	TenantOf() string
}

func (a *Agent) TenantOf() string          { return a.TenantID }
func (u *UnclaimedAgent) TenantOf() string { return u.TenantID }
func (s *Site) TenantOf() string           { return s.TenantID }
func (t *TestDef) TenantOf() string        { return t.TenantID }
func (r *AlertRule) TenantOf() string      { return r.TenantID }
func (t *AlertTarget) TenantOf() string    { return t.TenantID }
