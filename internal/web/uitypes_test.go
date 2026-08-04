package web

import (
	"os"
	"regexp"
	"testing"

	"github.com/fox27374/net-lama/internal/testtype"
)

// TestEveryTestTypeHasUIEntry keeps app.js's UI_TYPES table in step with
// the server's registry. The browser owns the half of a test type a server
// cannot serve — its form fields, how its params read and write, how its
// results read — and a type registered without an entry there renders
// blanks in the tests table and an empty dialog instead of failing.
func TestEveryTestTypeHasUIEntry(t *testing.T) {
	src, err := os.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("reading app.js: %v", err)
	}

	block := regexp.MustCompile(`(?s)const UI_TYPES = \{.*?\n\};`).Find(src)
	if block == nil {
		t.Fatal("no UI_TYPES table in app.js — was it renamed?")
	}

	inUI := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^  ([a-z_]+): \{`).FindAllSubmatch(block, -1) {
		inUI[string(m[1])] = true
	}

	for _, spec := range testtype.All() {
		if !inUI[spec.Type] {
			t.Errorf("test type %q has no UI_TYPES entry in app.js", spec.Type)
		}
		delete(inUI, spec.Type)
	}
	for name := range inUI {
		t.Errorf("UI_TYPES has %q, which the server does not register", name)
	}
}
