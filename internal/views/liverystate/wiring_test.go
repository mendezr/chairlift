package liverystate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The Livery page's handlers import puregotk, so a headless test cannot execute
// them. This reads the source instead and pins the two properties the pure
// transitions depend on: every asynchronous attempt resolves through this
// package before touching confirmed state, and every UI publish re-checks that
// its attempt is still the section's newest.
//
// Before the fix the handlers assigned uh.liveryState and the summary rows on
// the main thread before the work ran and had no generation re-check, so a
// failed fetch or a --dry-run preview stayed on screen. This test fails on
// that source the same way the pure tests fail on the old decision rules.
func TestLiveryHandlersResolveThroughPureTransitions(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	viewsDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
	path := filepath.Join(viewsDir, "livery_actions.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(data)

	for _, required := range []string{
		// Each action kind resolves through the pure transition table.
		"liverystate.Selection(",
		"liverystate.Toggle(",
		"liverystate.Rotation(",
		// A completed selection publishes only while it is still the newest
		// attempt for that surface.
		"serializer.IsCurrent(generation)",
		"uh.publishLiverySelection(",
		// Rotation is its own serializer, so its completion checks that one.
		"uh.liveryRotateWork.IsCurrent(generation)",
		// The candidate's artwork source is built from the candidate id, not
		// from the confirmed liveryState the handler has not committed yet.
		"liverySelectionSource(surface, id)",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("livery_actions.go does not contain %q", required)
		}
	}
}
