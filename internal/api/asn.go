package api

import (
	"net/http"
	"strings"

	"github.com/fox27374/net-lama/internal/asn"
	"github.com/fox27374/net-lama/internal/store"
)

// handleASNLookup resolves IP addresses to the network announcing them from
// the embedded routing table. GET /api/v1/asn?ips=1.1.1.1,8.8.8.8
// Returns {"1.1.1.1": {"asn":13335,"owner":"Cloudflare, Inc.","country":"US"}}
// with unannounced addresses omitted — the private hops at the start of a
// trace have no AS, which is an answer rather than an error.
//
// Batched because a path view resolves a whole trace at once; like
// /api/v1/oui and /api/v1/test-types it describes the internet rather than
// anyone's data, so it needs authentication but no tenant scoping.
func (a *API) handleASNLookup(w http.ResponseWriter, r *http.Request, _ *store.User) {
	out := map[string]asn.Info{}
	for _, ip := range strings.Split(r.URL.Query().Get("ips"), ",") {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if info, ok := asn.Lookup(ip); ok {
			out[ip] = info
		}
	}
	writeJSON(w, http.StatusOK, out)
}
