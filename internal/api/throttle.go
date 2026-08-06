package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	maxAuthFailures = 10
	authWindow      = time.Minute
)

// throttle counts failed authentication attempts per username+client IP and
// blocks further tries once a caller burns through maxAuthFailures inside
// authWindow. Keyed on both so one noisy IP cannot lock a real user out, and
// guessing a password from many IPs still hits the per-IP budget.
//
// ponytail: one mutex over one map, entries pruned as they are touched — the
// budget is per process, so two server instances mean twice the attempts.
// Move it into SQLite if that ever matters.
type throttle struct {
	mu      sync.Mutex
	entries map[string]*throttleEntry
}

type throttleEntry struct {
	failures int
	first    time.Time
}

func newThrottle() *throttle {
	return &throttle{entries: map[string]*throttleEntry{}}
}

// allow reports whether another attempt may be made, and prunes entries whose
// window has passed.
func (t *throttle) allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for k, e := range t.entries {
		if now.Sub(e.first) > authWindow {
			delete(t.entries, k)
		}
	}
	e := t.entries[key]
	return e == nil || e.failures < maxAuthFailures
}

// fail records a failed attempt and reports whether that attempt used up the
// budget, so the caller can log the moment it trips.
func (t *throttle) fail(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	e := t.entries[key]
	if e == nil || now.Sub(e.first) > authWindow {
		e = &throttleEntry{first: now}
		t.entries[key] = e
	}
	e.failures++
	return e.failures == maxAuthFailures
}

// success clears the budget for a key that just authenticated.
func (t *throttle) success(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

// clientIP is the peer address, host only. There is no proxy in front of the
// server today, so X-Forwarded-For is deliberately not trusted: anyone could
// set it and get a fresh budget per request.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func throttleKey(username string, r *http.Request) string {
	return username + "|" + clientIP(r)
}
