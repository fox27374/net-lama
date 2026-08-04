package saas

import (
	"net/url"
	"strings"
	"testing"
)

// TestCatalogEndpointsAreWellFormed pins the two things a catalog entry can
// get wrong without anything failing at build time: a kind nothing runs,
// and a target in the wrong shape for its kind (an https entry that is not
// a URL, or a tcp entry missing its port).
func TestCatalogEndpointsAreWellFormed(t *testing.T) {
	for id, svc := range catalog {
		if svc.ID != id {
			t.Errorf("service %q has ID %q — the key is the id stored in tests", id, svc.ID)
		}
		if svc.Name == "" {
			t.Errorf("service %q has no display name", id)
		}
		if len(svc.Endpoints) == 0 {
			t.Errorf("service %q checks nothing", id)
		}
		for _, ep := range svc.Endpoints {
			switch ep.Kind {
			case KindHTTPS:
				u, err := url.Parse(ep.Target)
				if err != nil || u.Scheme != "https" || u.Host == "" {
					t.Errorf("%s: https endpoint %q is not an https URL", id, ep.Target)
				}
			case KindTCP:
				host, port, found := strings.Cut(ep.Target, ":")
				if !found || host == "" || port == "" {
					t.Errorf("%s: tcp endpoint %q is not host:port", id, ep.Target)
				}
			default:
				t.Errorf("%s: endpoint %q has kind %q, which no agent runs", id, ep.Target, ep.Kind)
			}
		}
	}
}

// TestKnownServiceIDsSurvive is the permanence rule with teeth: these ids
// have shipped, so stored tests reference them. Endpoints inside a service
// may change freely; removing or renaming an id breaks existing tests
// silently — validation would reject them and they would stop being
// pushed. Deleting a line here is a deliberate act, not a refactor.
func TestKnownServiceIDsSurvive(t *testing.T) {
	shipped := []string{
		"ms-teams", "m365", "webex", "zoom",
		"google-workspace", "aws", "azure", "gcp",
	}
	for _, id := range shipped {
		if _, ok := Get(id); !ok {
			t.Errorf("shipped service id %q is gone — stored tests reference it", id)
		}
	}
}
