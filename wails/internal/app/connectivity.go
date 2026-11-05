// Model connectivity tests (Preferences → model registry) — the GUI-facing
// forward.
//
// The logic lives in internal/service/connectivity (network probes against
// the model registry + BYOK custom models); this file only marshals
// arguments, so the generated bindings keep their exact method names and
// parameter order.
package app

import (
	"droidgear-wails/internal/service/connectivity"
)

// Type aliases — one definition, in the service.
type (
	ConnectionDiagnostics = connectivity.ConnectionDiagnostics
	ModelTestResult       = connectivity.ModelTestResult
	ConnectivitySummary   = connectivity.ConnectivitySummary
)

// TestModelConnection probes one registry/BYOK model (default mode).
func (a *App) TestModelConnection(modelID string) (ModelTestResult, error) {
	return connectivity.TestModelConnection(modelID)
}

// TestModelConnectionWithMode probes one model with an explicit ping or
// inference mode.
func (a *App) TestModelConnectionWithMode(modelID string, mode string, prompt *string) (ModelTestResult, error) {
	return connectivity.TestModelConnectionWithMode(modelID, mode, prompt)
}

// TestAllModelConnectionsCommand probes every configured model (default mode).
func (a *App) TestAllModelConnectionsCommand() ([]ModelTestResult, error) {
	return connectivity.TestAllModelConnectionsCommand()
}

// TestAllModelConnectionsWithMode probes every model with an explicit mode.
func (a *App) TestAllModelConnectionsWithMode(mode string, prompt *string) ([]ModelTestResult, error) {
	return connectivity.TestAllModelConnectionsWithMode(mode, prompt)
}

// TestProviderConnection probes a provider endpoint directly (no registry).
func (a *App) TestProviderConnection(provider string, baseUrl string, apiKey string, modelID string) (ConnectionDiagnostics, error) {
	return connectivity.TestProviderConnection(provider, baseUrl, apiKey, modelID)
}
