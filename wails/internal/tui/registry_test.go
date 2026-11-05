package tui

import (
	"context"
	"strings"
	"testing"

	"droidgear-wails/internal/testutil"
)

func TestRegistryScreenListsVendoredModels(t *testing.T) {
	screen, err := newListScreen(context.Background(), RegistrySource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if got := screen.Status(); !strings.Contains(got, "Registry · ") || !strings.Contains(got, "reasoning") {
		t.Fatalf("status = %q, want the registry summary", got)
	}
	view := screen.View(100, 30)
	if !strings.Contains(view, "GPT") && !strings.Contains(view, "Claude") {
		t.Fatalf("registry view looks empty:\n%s", view)
	}
	// Detail pane shows the selected registry entry.
	if got := screen.Status(); strings.Contains(got, "error") {
		t.Fatalf("registry screen errored: %s", got)
	}
}

// TestModelConnectivityActionDispatchesWithoutNetwork runs the connectivity
// view action against an empty BYOK list: the model lookup fails before any
// network I/O, which is exactly the dispatch path we want covered (the probe
// logic itself is covered by service/connectivity's httptest suite).
func TestModelConnectivityActionDispatchesWithoutNetwork(t *testing.T) {
	testutil.IsolatedHome(t)
	screen, err := newListScreen(context.Background(), FactorySource())
	if err != nil {
		t.Fatalf("newListScreen: %v", err)
	}
	if !strings.Contains(screen.Status(), "c test ping") {
		t.Fatalf("status = %q, want the connectivity hints", screen.Status())
	}
	// Empty list: the action still dispatches with a zero row and the
	// service reports "not found" without touching the network.
	next, _ := screen.Update(keyMsg("c"))
	updated, ok := next.(*listScreen)
	if !ok {
		t.Fatalf("c returned %T, want the list screen with a status", next)
	}
	if got := updated.Status(); !strings.Contains(got, "test ping failed") || !strings.Contains(got, "not found") {
		t.Fatalf("status = %q, want a not-found failure", got)
	}
}
