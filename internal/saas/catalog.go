// Package saas is the catalog of online services a `saas` test can check.
//
// A Service is a curated name ("Microsoft Teams") for the handful of
// endpoints that service actually depends on. Stored tests reference a
// service by id and nothing else: the server expands the id into endpoints
// on every config push, so fixing or extending a service ships with a
// server release and never needs the agent fleet updated.
//
// Service ids are permanent. Endpoints inside a service may be fixed,
// added or removed freely, but a shipped id is never renamed or removed —
// stored tests reference it, and validation rejects an unknown one.
//
// Endpoint kind is not a style choice. The server counts an HTTP result as
// OK only on 2xx/3xx (resultOK in internal/server), and a machine API that
// answers 401/403/400 to an unauthenticated GET is behaving correctly, not
// failing. So: https for user-facing front doors that answer 2xx/3xx, tcp
// for machine APIs that answer 4xx. Connect-only loses the status code and
// the TLS timings, but it never reports a healthy API as down.
//
// ponytail: a per-endpoint expected-status field would let the API hosts
// be checked over https properly. It needs resultOK to know the test type,
// which is possible now that the type comes from the test definition. Do
// it when someone wants TLS expiry or TTFB on the API front doors.
//
// Every endpoint below was verified live on 2026-08-04 and against the
// vendor's own documentation — Microsoft's machine-readable endpoint feed
// (endpoints.office.com), Webex's network requirements, Zoom's firewall
// article, and Google Workspace's firewall and proxy settings. Vendor
// status pages are deliberately absent: they answer 200 from separate CDN
// infrastructure and say nothing about whether the service works here.
package saas

import "sort"

// Endpoint kinds.
const (
	KindHTTPS = "https"
	KindTCP   = "tcp"
)

// Endpoint is one thing to check for a service: a URL to GET (https) or a
// host:port to connect to (tcp).
type Endpoint struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

// Service is one catalog entry.
type Service struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Endpoints []Endpoint `json:"endpoints"`
}

func https(url string) Endpoint { return Endpoint{Kind: KindHTTPS, Target: url} }
func tcp(target string) Endpoint {
	return Endpoint{Kind: KindTCP, Target: target}
}

// catalog is the service registry, keyed by permanent service id.
var catalog = map[string]Service{
	"ms-teams": {
		ID:   "ms-teams",
		Name: "Microsoft Teams",
		// Teams media (UDP 3478-3481) is not checked: Microsoft publishes
		// it as IP ranges with no hostnames, so there is nothing to
		// resolve and no TCP fallback that would mean anything. A green
		// ms-teams test means sign-in and signalling work, not that a
		// call will.
		Endpoints: []Endpoint{
			https("https://teams.microsoft.com"),
			https("https://teams.cloud.microsoft"),
			https("https://login.microsoftonline.com"),
		},
	},
	"m365": {
		ID:   "m365",
		Name: "Microsoft 365",
		Endpoints: []Endpoint{
			https("https://outlook.office365.com"),
			https("https://graph.microsoft.com"),
			https("https://login.microsoftonline.com"),
		},
	},
	"webex": {
		ID:   "webex",
		Name: "Cisco Webex",
		Endpoints: []Endpoint{
			https("https://web.webex.com"),
			// The API root answers 401 unauthenticated; /v1/ping is the
			// unauthenticated health endpoint and answers 200.
			https("https://webexapis.com/v1/ping"),
		},
	},
	"zoom": {
		ID:   "zoom",
		Name: "Zoom",
		Endpoints: []Endpoint{
			https("https://zoom.us"),
			https("https://api.zoom.us"),
		},
	},
	"google-workspace": {
		ID:   "google-workspace",
		Name: "Google Workspace",
		Endpoints: []Endpoint{
			https("https://mail.google.com"),
			https("https://accounts.google.com"),
			https("https://drive.google.com"),
		},
	},
	"aws": {
		ID:   "aws",
		Name: "Amazon Web Services",
		Endpoints: []Endpoint{
			https("https://console.aws.amazon.com"),
			https("https://signin.aws.amazon.com"),
			tcp("ec2.amazonaws.com:443"),
		},
	},
	"azure": {
		ID:   "azure",
		Name: "Microsoft Azure",
		Endpoints: []Endpoint{
			https("https://login.microsoftonline.com"),
			// Both answer 4xx to an unauthenticated GET by design.
			tcp("portal.azure.com:443"),
			tcp("management.azure.com:443"),
		},
	},
	"gcp": {
		ID:   "gcp",
		Name: "Google Cloud",
		Endpoints: []Endpoint{
			https("https://console.cloud.google.com"),
			tcp("storage.googleapis.com:443"),
		},
	},
}

// Get returns the service with this id, or ok=false if the catalog has no
// such service.
func Get(id string) (Service, bool) {
	s, ok := catalog[id]
	return s, ok
}

// All returns every service, sorted by display name for a stable dropdown.
func All() []Service {
	out := make([]Service, 0, len(catalog))
	for _, s := range catalog {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
