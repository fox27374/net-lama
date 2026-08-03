package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/fox27374/net-lama/internal/server"
	"github.com/fox27374/net-lama/internal/store"
)

// tenantFixture is one tenant's worth of rows, so a test can address any
// tenant-scoped resource by ID.
type tenantFixture struct {
	tenant    *store.Tenant
	user      *store.User
	session   string
	site      *store.Site
	agent     *store.Agent
	test      *store.TestDef
	rule      *store.AlertRule
	target    *store.AlertTarget
	unclaimed *store.UnclaimedAgent
	apiKey    *store.APIKey
}

type harness struct {
	st    *store.Store
	mux   *http.ServeMux
	a, b  *tenantFixture
	admin *tenantFixture // admin user, no tenant of its own
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "scope-test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := server.New(st, server.NewMetrics(prometheus.NewRegistry()), logger)

	mux := http.NewServeMux()
	New(st, srv, logger, false).Register(mux)

	h := &harness{st: st, mux: mux}
	h.a = h.seedTenant(t, "tenant-a")
	h.b = h.seedTenant(t, "tenant-b")

	adminUser, err := st.CreateUser("", "root", "pw", true)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	adminSession, err := st.CreateSession(adminUser.ID)
	if err != nil {
		t.Fatalf("admin session: %v", err)
	}
	h.admin = &tenantFixture{user: adminUser, session: adminSession}
	return h
}

func (h *harness) seedTenant(t *testing.T, name string) *tenantFixture {
	t.Helper()
	f := &tenantFixture{}
	var err error

	if f.tenant, err = h.st.CreateTenant(name); err != nil {
		t.Fatalf("create tenant %s: %v", name, err)
	}
	if f.user, err = h.st.CreateUser(f.tenant.ID, "user-"+name, "pw", false); err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	if f.session, err = h.st.CreateSession(f.user.ID); err != nil {
		t.Fatalf("create session %s: %v", name, err)
	}
	if f.site, err = h.st.CreateSite(f.tenant.ID, "site-"+name); err != nil {
		t.Fatalf("create site %s: %v", name, err)
	}
	if f.agent, err = h.st.CreateAgent(f.tenant.ID, f.site.ID, "agent-"+name); err != nil {
		t.Fatalf("create agent %s: %v", name, err)
	}
	f.test, err = h.st.CreateTest(&store.TestDef{
		TenantID:        f.tenant.ID,
		Name:            "ping-" + name,
		Type:            "ping",
		IntervalSeconds: 60,
		Params:          json.RawMessage(`{"targets":["8.8.8.8"],"count":5}`),
	})
	if err != nil {
		t.Fatalf("create test %s: %v", name, err)
	}
	if err := h.st.SetSiteTests(f.site.ID, []string{f.test.ID}); err != nil {
		t.Fatalf("assign test %s: %v", name, err)
	}
	f.target, err = h.st.CreateAlertTarget(&store.AlertTarget{
		TenantID: f.tenant.ID,
		Name:     "hook-" + name,
		Type:     "webhook",
		Config:   map[string]any{"url": "http://example.invalid/hook"},
	})
	if err != nil {
		t.Fatalf("create target %s: %v", name, err)
	}
	f.rule, err = h.st.CreateAlertRule(&store.AlertRule{
		TenantID:  f.tenant.ID,
		TestID:    f.test.ID,
		Name:      "rule-" + name,
		Metric:    "latency_ms",
		Operator:  ">",
		Threshold: 100,
	})
	if err != nil {
		t.Fatalf("create rule %s: %v", name, err)
	}
	if err := h.st.UpsertUnclaimedAgent(f.tenant.ID, "client-"+name, "v1", nil, nil); err != nil {
		t.Fatalf("upsert unclaimed %s: %v", name, err)
	}
	unclaimed, err := h.st.ListUnclaimedAgents(f.tenant.ID)
	if err != nil || len(unclaimed) != 1 {
		t.Fatalf("list unclaimed %s: %v (%d rows)", name, err, len(unclaimed))
	}
	f.unclaimed = unclaimed[0]
	if f.apiKey, err = h.st.CreateAPIKey(f.user.ID, "key-"+name); err != nil {
		t.Fatalf("create api key %s: %v", name, err)
	}
	return f
}

// do issues a request authenticated as the given fixture's user.
func (h *harness) do(t *testing.T, as *tenantFixture, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: as.session})
	w := httptest.NewRecorder()
	h.mux.ServeHTTP(w, r)
	return w
}

// idRoute is one route that addresses a resource by ID in its path. Every
// such route registered in api.go must appear here — TestEveryIDRouteIsScoped
// fails if one is added without a cross-tenant case.
type idRoute struct {
	method string
	// pattern is the route as registered in api.go, used for the coverage check.
	pattern string
	// path builds the concrete request path against another tenant's rows.
	path func(other *tenantFixture) string
	body string
	// want is the status a tenant user must get for another tenant's ID.
	want int
}

func idRoutes() []idRoute {
	return []idRoute{
		// Admin-only routes: a tenant user is stopped by the admin middleware.
		{"DELETE", "DELETE /api/v1/tenants/{id}",
			func(o *tenantFixture) string { return "/api/v1/tenants/" + o.tenant.ID }, "", http.StatusForbidden},
		{"DELETE", "DELETE /api/v1/users/{id}",
			func(o *tenantFixture) string { return "/api/v1/users/" + o.user.ID }, "", http.StatusForbidden},

		// Tenant-scoped resources: another tenant's ID must look absent.
		{"DELETE", "DELETE /api/v1/sites/{id}",
			func(o *tenantFixture) string { return "/api/v1/sites/" + o.site.ID }, "", http.StatusNotFound},
		{"PUT", "PUT /api/v1/sites/{id}/tests",
			func(o *tenantFixture) string { return "/api/v1/sites/" + o.site.ID + "/tests" }, `{"testIds":[]}`, http.StatusNotFound},
		{"PUT", "PUT /api/v1/tests/{id}",
			func(o *tenantFixture) string { return "/api/v1/tests/" + o.test.ID }, `{"name":"x","intervalSeconds":60}`, http.StatusNotFound},
		{"DELETE", "DELETE /api/v1/tests/{id}",
			func(o *tenantFixture) string { return "/api/v1/tests/" + o.test.ID }, "", http.StatusNotFound},
		{"PUT", "PUT /api/v1/agents/{id}",
			func(o *tenantFixture) string { return "/api/v1/agents/" + o.agent.ID }, `{"name":"x","siteId":"y"}`, http.StatusNotFound},
		{"POST", "POST /api/v1/agents/{id}/run",
			func(o *tenantFixture) string { return "/api/v1/agents/" + o.agent.ID + "/run" }, `{"testId":"x"}`, http.StatusNotFound},
		{"DELETE", "DELETE /api/v1/agents/{id}",
			func(o *tenantFixture) string { return "/api/v1/agents/" + o.agent.ID }, "", http.StatusNotFound},
		{"POST", "POST /api/v1/agents/unclaimed/{id}/claim",
			func(o *tenantFixture) string { return "/api/v1/agents/unclaimed/" + o.unclaimed.ID + "/claim" }, `{"name":"x","siteId":"y"}`, http.StatusNotFound},
		{"DELETE", "DELETE /api/v1/agents/unclaimed/{id}",
			func(o *tenantFixture) string { return "/api/v1/agents/unclaimed/" + o.unclaimed.ID }, "", http.StatusNotFound},
		{"PUT", "PUT /api/v1/alert-rules/{id}",
			func(o *tenantFixture) string { return "/api/v1/alert-rules/" + o.rule.ID }, `{"name":"x","metric":"latency_ms","operator":">","threshold":1}`, http.StatusNotFound},
		{"DELETE", "DELETE /api/v1/alert-rules/{id}",
			func(o *tenantFixture) string { return "/api/v1/alert-rules/" + o.rule.ID }, "", http.StatusNotFound},
		{"PUT", "PUT /api/v1/alert-targets/{id}",
			func(o *tenantFixture) string { return "/api/v1/alert-targets/" + o.target.ID }, `{"name":"x","type":"webhook","config":{"url":"http://e.invalid"}}`, http.StatusNotFound},
		{"DELETE", "DELETE /api/v1/alert-targets/{id}",
			func(o *tenantFixture) string { return "/api/v1/alert-targets/" + o.target.ID }, "", http.StatusNotFound},

		// API keys are scoped per user rather than per tenant, but the
		// property under test is the same: another owner's ID is absent.
		{"DELETE", "DELETE /api/v1/apikeys/{id}",
			func(o *tenantFixture) string { return "/api/v1/apikeys/" + o.apiKey.ID }, "", http.StatusNotFound},
	}
}

// TestCrossTenantIDsAreNotFound is the regression guard for the whole
// package: a tenant user addressing another tenant's row by ID must never
// succeed, and must not be able to tell the row apart from one that does
// not exist (403 would confirm it does).
func TestCrossTenantIDsAreNotFound(t *testing.T) {
	h := newHarness(t)

	for _, rt := range idRoutes() {
		t.Run(rt.pattern, func(t *testing.T) {
			w := h.do(t, h.a, rt.method, rt.path(h.b), rt.body)
			if w.Code >= 200 && w.Code < 300 {
				t.Fatalf("cross-tenant request succeeded: %s → %d %s", rt.pattern, w.Code, w.Body.String())
			}
			if w.Code != rt.want {
				t.Errorf("%s: status = %d, want %d (body: %s)", rt.pattern, w.Code, rt.want, w.Body.String())
			}
		})
	}
}

// TestOwnTenantIDsAreReachable is the positive control for the test above:
// without it, a harness that rejected everything would pass vacuously.
func TestOwnTenantIDsAreReachable(t *testing.T) {
	h := newHarness(t)

	cases := []struct {
		name, method, path, body string
	}{
		{"update own test", "PUT", "/api/v1/tests/" + h.a.test.ID,
			`{"name":"renamed","intervalSeconds":60,"params":{"targets":["1.1.1.1"],"count":3}}`},
		{"update own agent", "PUT", "/api/v1/agents/" + h.a.agent.ID,
			`{"name":"renamed","siteId":"` + h.a.site.ID + `"}`},
		{"update own alert target", "PUT", "/api/v1/alert-targets/" + h.a.target.ID,
			`{"name":"renamed","type":"webhook","config":{"url":"http://example.invalid/x"}}`},
		{"assign own tests to own site", "PUT", "/api/v1/sites/" + h.a.site.ID + "/tests",
			`{"testIds":["` + h.a.test.ID + `"]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := h.do(t, h.a, c.method, c.path, c.body)
			if w.Code < 200 || w.Code >= 300 {
				t.Fatalf("status = %d, want 2xx (body: %s)", w.Code, w.Body.String())
			}
		})
	}
}

// TestBodyReferencedIDsAreTenantChecked covers the other half of the seam:
// IDs that arrive in a request body rather than the path.
func TestBodyReferencedIDsAreTenantChecked(t *testing.T) {
	h := newHarness(t)

	cases := []struct{ name, method, path, body string }{
		{"create agent in another tenant's site", "POST", "/api/v1/agents",
			`{"name":"x","siteId":"` + h.b.site.ID + `"}`},
		{"assign another tenant's test to own site", "PUT", "/api/v1/sites/" + h.a.site.ID + "/tests",
			`{"testIds":["` + h.b.test.ID + `"]}`},
		{"alert rule on another tenant's test", "POST", "/api/v1/alert-rules",
			`{"name":"x","testId":"` + h.b.test.ID + `","metric":"latency_ms","operator":">","threshold":1}`},
		{"alert rule with another tenant's target", "POST", "/api/v1/alert-rules",
			`{"name":"x","testId":"` + h.a.test.ID + `","metric":"latency_ms","operator":">","threshold":1,"targetIds":["` + h.b.target.ID + `"]}`},
		{"overview filtered by another tenant's site", "GET",
			"/api/v1/overview?siteId=" + h.b.site.ID, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := h.do(t, h.a, c.method, c.path, c.body)
			if w.Code >= 200 && w.Code < 300 {
				t.Fatalf("accepted another tenant's ID: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestListEndpointsAreTenantScoped checks the collection endpoints, which
// scope by filtering the query rather than by rejecting an ID.
func TestListEndpointsAreTenantScoped(t *testing.T) {
	h := newHarness(t)

	for _, path := range []string{"/api/v1/sites", "/api/v1/tests", "/api/v1/agents", "/api/v1/alert-rules", "/api/v1/alert-targets", "/api/v1/agents/unclaimed"} {
		t.Run(path, func(t *testing.T) {
			// Without a tenantId, and with another tenant's tenantId asked
			// for explicitly, a tenant user sees only its own rows.
			for _, query := range []string{"", "?tenantId=" + h.b.tenant.ID} {
				w := h.do(t, h.a, "GET", path+query, "")
				if w.Code >= 200 && w.Code < 300 {
					if strings.Contains(w.Body.String(), h.b.tenant.ID) {
						t.Fatalf("%s%s leaked tenant-b rows: %s", path, query, w.Body.String())
					}
				} else if query == "" {
					t.Fatalf("%s: status = %d, want 2xx (body: %s)", path, w.Code, w.Body.String())
				}
			}
		})
	}
}

// TestEveryIDRouteIsScoped is the conformance guard that keeps the seam
// from eroding: it reads the route table in api.go and fails when a route
// takes an {id} but has no cross-tenant case in idRoutes().
func TestEveryIDRouteIsScoped(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("reading api.go: %v", err)
	}

	covered := map[string]bool{}
	for _, rt := range idRoutes() {
		covered[rt.pattern] = true
	}

	registered := regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+ /api/v1/[^"]*\{id\}[^"]*)"`)
	found := 0
	for _, m := range registered.FindAllStringSubmatch(string(src), -1) {
		found++
		if !covered[m[1]] {
			t.Errorf("route %q takes an {id} but has no case in idRoutes() — add one so cross-tenant access stays tested", m[1])
		}
	}
	if found != len(covered) {
		t.Errorf("api.go registers %d {id} routes, idRoutes() lists %d — a listed route may have been removed or renamed", found, len(covered))
	}
}

func TestTenantScope(t *testing.T) {
	admin := &store.User{IsAdmin: true}
	member := &store.User{TenantID: "t1"}

	cases := []struct {
		name      string
		user      *store.User
		requested string
		want      string
		wantOK    bool
	}{
		{"admin must name a tenant", admin, "", "", false},
		{"admin may name any tenant", admin, "t9", "t9", true},
		{"member defaults to own tenant", member, "", "t1", true},
		{"member may name own tenant", member, "t1", "t1", true},
		{"member may not name another tenant", member, "t2", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := tenantScope(c.user, c.requested)
			if got != c.want || ok != c.wantOK {
				t.Errorf("tenantScope(%q) = (%q, %v), want (%q, %v)", c.requested, got, ok, c.want, c.wantOK)
			}
		})
	}
}

func TestTenantFilter(t *testing.T) {
	admin := &store.User{IsAdmin: true}
	member := &store.User{TenantID: "t1"}

	// The one difference from tenantScope: an admin naming no tenant lists
	// every tenant instead of erroring.
	if got, ok := tenantFilter(admin, ""); got != "" || !ok {
		t.Errorf(`tenantFilter(admin, "") = (%q, %v), want ("", true)`, got, ok)
	}
	if _, ok := tenantFilter(member, "t2"); ok {
		t.Error("tenantFilter let a member list another tenant")
	}
	if got, ok := tenantFilter(member, ""); got != "t1" || !ok {
		t.Errorf(`tenantFilter(member, "") = (%q, %v), want ("t1", true)`, got, ok)
	}
}
