package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Profile CRUD round trip
// ---------------------------------------------------------------------------

// TestOpencodeProfileCRUDAndApplyRoundTrip mirrors the mount sequence the
// OpenCode page runs: create default → list → update (keeping createdAt) →
// duplicate → apply (merging into the live config) → delete clearing the
// active marker.
func TestOpencodeProfileCRUDAndApplyRoundTrip(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	created, err := a.CreateDefaultProfile()
	if err != nil {
		t.Fatalf("CreateDefaultProfile: %v", err)
	}
	if created.Name != "Default" {
		t.Fatalf("default name wrong: %+v", created)
	}
	active, err := a.GetActiveOpencodeProfileId()
	if err != nil || active == nil || *active != created.ID {
		t.Fatalf("create must mark the profile active: %v / %v", active, err)
	}

	profiles, err := a.ListOpencodeProfiles()
	if err != nil || len(profiles) != 1 {
		t.Fatalf("ListOpencodeProfiles: %v (n=%d)", err, len(profiles))
	}

	// Update: createdAt must survive, updatedAt must advance.
	created.Name = "Renamed"
	created.Description = strPtr("desc")
	created.Providers = map[string]OpenCodeProviderConfig{
		"anthropic": {NPM: strPtr("@ai-sdk/anthropic")},
	}
	if err := a.SaveOpencodeProfile(created); err != nil {
		t.Fatalf("SaveOpencodeProfile: %v", err)
	}
	loaded, err := a.GetOpencodeProfile(created.ID)
	if err != nil {
		t.Fatalf("GetOpencodeProfile: %v", err)
	}
	if loaded.CreatedAt != created.CreatedAt {
		t.Fatalf("createdAt must be preserved: %q vs %q", loaded.CreatedAt, created.CreatedAt)
	}
	if loaded.Name != "Renamed" || loaded.Providers["anthropic"].NPM == nil {
		t.Fatalf("update lost: %+v", loaded)
	}

	// Duplicate gets a fresh id, keeps api-bearing provider config.
	dup, err := a.DuplicateOpencodeProfile(created.ID, "Copy")
	if err != nil {
		t.Fatalf("DuplicateOpencodeProfile: %v", err)
	}
	if dup.ID == created.ID || dup.Name != "Copy" {
		t.Fatalf("duplicate wrong: %+v", dup)
	}
	if profiles, _ = a.ListOpencodeProfiles(); len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}

	// Apply writes providers + auth into the live files and keeps unknown
	// config fields (serde Value round-trip parity).
	status, err := a.GetOpencodeConfigStatus()
	if err != nil {
		t.Fatalf("GetOpencodeConfigStatus: %v", err)
	}
	writeTestFile(t, status.ConfigPath, `{
  // jsonc comment
  "theme": "dark",
  "unknownTopLevel": {"keep": true},
  "provider": {"existing": {"npm": "@ai/old"}}
}`)
	writeTestFile(t, status.AuthPath, `{"openai": {"type": "api", "key": "sk-from-auth"}}`)

	applyProfile := loaded
	applyProfile.Providers = map[string]OpenCodeProviderConfig{
		"anthropic": {NPM: strPtr("@ai-sdk/anthropic"), Options: &OpenCodeProviderOptions{BaseURL: strPtr("https://api.anthropic.com/v1")}},
	}
	applyProfile.Auth = map[string]any{"anthropic": map[string]any{"type": "api", "key": "sk-live"}}
	if err := a.SaveOpencodeProfile(applyProfile); err != nil {
		t.Fatalf("SaveOpencodeProfile: %v", err)
	}
	if err := a.ApplyOpencodeProfile(applyProfile.ID); err != nil {
		t.Fatalf("ApplyOpencodeProfile: %v", err)
	}

	config := mustRead(t, status.ConfigPath)
	if strings.Contains(config, "jsonc comment") {
		// Rust strips comments on read and rewrites the document.
		t.Fatalf("comments should be stripped on apply: %s", config)
	}
	for _, want := range []string{`"theme": "dark"`, `"unknownTopLevel"`, `"existing"`, `"anthropic"`} {
		if !strings.Contains(config, want) {
			t.Fatalf("apply lost %s: %s", want, config)
		}
	}
	auth := mustRead(t, status.AuthPath)
	if !strings.Contains(auth, "sk-from-auth") || !strings.Contains(auth, "sk-live") {
		t.Fatalf("auth merge lost entries: %s", auth)
	}
	active, err = a.GetActiveOpencodeProfileId()
	if err != nil || active == nil || *active != applyProfile.ID {
		t.Fatalf("apply must record the active id: %v / %v", active, err)
	}

	// Read current config: baseUrl → baseURL normalisation + apiKey → auth.
	readTestFile(t, status.ConfigPath, `{
  "provider": {
    "anthropic": {"npm": "@ai-sdk/anthropic", "options": {"baseUrl": "https://api.anthropic.com/v1", "apiKey": "sk-inline"}},
    "openai": {"npm": "@ai-sdk/openai", "options": {"baseURL": "https://api.openai.com/v1"}}
  }
}`)
	readTestFile(t, status.AuthPath, `{"openai": {"type": "api", "key": "sk-from-auth"}}`)
	current, err := a.ReadOpencodeCurrentConfig()
	if err != nil {
		t.Fatalf("ReadOpencodeCurrentConfig: %v", err)
	}
	if current.Providers["anthropic"].Options == nil || current.Providers["anthropic"].Options.BaseURL == nil {
		t.Fatalf("baseUrl not normalised to baseURL: %+v", current.Providers)
	}
	if current.Auth["openai"] == nil || current.Auth["anthropic"] == nil {
		t.Fatalf("auth missing entries: %+v", current.Auth)
	}
	openaiAuth, _ := current.Auth["openai"].(map[string]any)
	if openaiAuth["key"] != "sk-from-auth" {
		t.Fatalf("auth file entry must win: %+v", openaiAuth)
	}
	anthropicAuth, _ := current.Auth["anthropic"].(map[string]any)
	if anthropicAuth["type"] != "api" || anthropicAuth["key"] != "sk-inline" {
		t.Fatalf("options.apiKey must be synthesised into auth: %+v", anthropicAuth)
	}

	// Delete clears the active marker when it points at the deleted profile.
	if err := a.DeleteOpencodeProfile(applyProfile.ID); err != nil {
		t.Fatalf("DeleteOpencodeProfile: %v", err)
	}
	active, err = a.GetActiveOpencodeProfileId()
	if err != nil || active != nil {
		t.Fatalf("delete must clear the active id: %v / %v", active, err)
	}
	if profiles, _ = a.ListOpencodeProfiles(); len(profiles) != 1 {
		t.Fatalf("expected 1 profile after delete, got %d", len(profiles))
	}
}

// TestOpencodeCreateDefaultOnlyWhenEmpty pins the "Profiles already exist"
// guard used by the store's loadProfiles fallback.
func TestOpencodeCreateDefaultOnlyWhenEmpty(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	if _, err := a.CreateDefaultProfile(); err != nil {
		t.Fatalf("first CreateDefaultProfile: %v", err)
	}
	_, err := a.CreateDefaultProfile()
	if err == nil || err.Error() != "Profiles already exist" {
		t.Fatalf("second create must fail with the known error, got %v", err)
	}
}

// TestOpencodeInvalidProfileIDRejected covers the shared id validator: ids
// are ascii alphanumerics plus '-' and '_', non-empty.
func TestOpencodeInvalidProfileIDRejected(t *testing.T) {
	isolatedHome(t)
	a := &App{}

	if _, err := a.GetOpencodeProfile("../evil"); err == nil || err.Error() != "Invalid profile id" {
		t.Fatalf("expected Invalid profile id, got %v", err)
	}
	if _, err := a.GetOpencodeProfile(""); err == nil || err.Error() != "Invalid profile id" {
		t.Fatalf("expected Invalid profile id for empty id, got %v", err)
	}
	// A valid-looking id with a missing file surfaces the read error instead.
	if _, err := a.GetOpencodeProfile("nope"); err == nil || !strings.Contains(err.Error(), "Failed to read profile") {
		t.Fatalf("expected read failure, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Config status / paths overrides
// ---------------------------------------------------------------------------

// TestOpencodeConfigStatusHonoursPathOverride checks Preferences → Paths:
// configPaths.opencode / configPaths.opencodeAuth in ~/.droidgear/settings.json
// must redirect the status paths, and the .jsonc file wins over .json.
func TestOpencodeConfigStatusHonoursPathOverride(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}

	customConfig := filepath.Join(t.TempDir(), "oc-config")
	customAuth := filepath.Join(t.TempDir(), "oc-auth")
	writeTestFile(t, filepath.Join(home, ".droidgear", "settings.json"),
		`{"configPaths": {"opencode": `+quoteJSON(customConfig)+`, "opencodeAuth": `+quoteJSON(customAuth)+`}}`)

	status, err := a.GetOpencodeConfigStatus()
	if err != nil {
		t.Fatalf("GetOpencodeConfigStatus: %v", err)
	}
	if status.ConfigPath != filepath.Join(customConfig, "opencode.json") {
		t.Fatalf("config path override ignored: %s", status.ConfigPath)
	}
	if status.AuthPath != filepath.Join(customAuth, "auth.json") {
		t.Fatalf("auth path override ignored: %s", status.AuthPath)
	}
	if status.ConfigExists || status.AuthExists {
		t.Fatalf("fresh override dirs must report missing files: %+v", status)
	}

	// .jsonc wins over .json when both exist.
	writeTestFile(t, filepath.Join(customConfig, "opencode.json"), `{}`)
	writeTestFile(t, filepath.Join(customConfig, "opencode.jsonc"), `{}`)
	status, err = a.GetOpencodeConfigStatus()
	if err != nil {
		t.Fatalf("GetOpencodeConfigStatus (2): %v", err)
	}
	if !strings.HasSuffix(status.ConfigPath, "opencode.jsonc") || !status.ConfigExists {
		t.Fatalf("jsonc must win: %+v", status)
	}
}

// ---------------------------------------------------------------------------
// Provider templates & connection test
// ---------------------------------------------------------------------------

func TestOpencodeProviderTemplates(t *testing.T) {
	a := &App{}
	templates, err := a.GetOpencodeProviderTemplates()
	if err != nil {
		t.Fatalf("GetOpencodeProviderTemplates: %v", err)
	}
	if len(templates) != 3 {
		t.Fatalf("expected 3 templates, got %d", len(templates))
	}
	anthropic := templates[0]
	if anthropic.ID != "anthropic" || anthropic.NPM == nil || *anthropic.NPM != "@ai-sdk/anthropic" ||
		anthropic.DefaultBaseURL == nil || *anthropic.DefaultBaseURL != "https://api.anthropic.com/v1" ||
		!anthropic.RequiresAPIKey {
		t.Fatalf("anthropic template wrong: %+v", anthropic)
	}
	if templates[1].ID != "openai" || templates[2].ID != "gemini" {
		t.Fatalf("template order wrong: %+v", templates)
	}
}

func TestOpencodeTestProviderConnection(t *testing.T) {
	var gotPath, gotAPIKey, gotAnthropicVersion, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotAnthropicVersion = r.Header.Get("anthropic-version")
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/v1/models" && r.Header.Get("x-api-key") == "sk-ok" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	a := &App{}
	ok, err := a.TestOpencodeProviderConnection("anthropic", server.URL+"/", "sk-ok")
	if err != nil {
		t.Fatalf("TestOpencodeProviderConnection: %v", err)
	}
	if !ok {
		t.Fatalf("expected success for 200 response")
	}
	// base_url trailing slash must be trimmed before appending /v1/models.
	if gotPath != "/v1/models" || gotAPIKey != "sk-ok" || gotAnthropicVersion != "2023-06-01" {
		t.Fatalf("anthropic probe wrong: path=%q apiKey=%q version=%q", gotPath, gotAPIKey, gotAnthropicVersion)
	}

	ok, err = a.TestOpencodeProviderConnection("openai", server.URL, "sk-bad")
	if err != nil {
		t.Fatalf("TestOpencodeProviderConnection (openai): %v", err)
	}
	if ok {
		t.Fatalf("401 must not report success")
	}
	if gotAuth != "Bearer sk-bad" || gotAPIKey != "" {
		t.Fatalf("openai probe must use Bearer auth: auth=%q", gotAuth)
	}

	// Connection failures surface the Rust "Connection failed:" prefix.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()
	if _, err := a.TestOpencodeProviderConnection("openai", dead.URL, "sk"); err == nil || !strings.HasPrefix(err.Error(), "Connection failed: ") {
		t.Fatalf("expected Connection failed error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// JSONC reading
// ---------------------------------------------------------------------------

// TestOpencodeReadJSONStripsComments pins the json_comments::StripComments
// parity: //, /* */ and strings containing slashes are handled correctly;
// unreadable or unparsable documents fall back to {}.
func TestOpencodeReadJSONStripsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	writeTestFile(t, path, `{
  // line comment with "quotes" and https://slash
  "a": 1, /* block */ "b": {"url": "https://example.test//not-a-comment"}
}`)
	value, ok := opencodeReadJSONValue(path).(map[string]any)
	if !ok {
		t.Fatalf("expected object root")
	}
	if value["a"] == nil || value["b"] == nil {
		t.Fatalf("comment stripping broke parsing: %+v", value)
	}
	nested, _ := value["b"].(map[string]any)
	if nested["url"] != "https://example.test//not-a-comment" {
		t.Fatalf("string content must be untouched: %+v", nested)
	}

	// Missing / invalid documents behave like json!({}).
	if got := opencodeReadJSONValue(filepath.Join(dir, "absent.json")); got == nil {
		t.Fatalf("missing file must yield an empty object")
	}
	writeTestFile(t, filepath.Join(dir, "broken.json"), "{not json")
	if got, ok := opencodeReadJSONValue(filepath.Join(dir, "broken.json")).(map[string]any); !ok || len(got) != 0 {
		t.Fatalf("invalid file must yield an empty object, got %#v", got)
	}
}

// TestOpencodeMarshalJSONKeepsUnknownFieldsRoundTrip: numbers keep their
// literal form (json.Number) so rewriting a config never reorders or
// reformats values.
func TestOpencodeMarshalJSONKeepsNumbers(t *testing.T) {
	value := map[string]any{
		"timeout": json.Number("1500"),
		"ratio":   json.Number("1.50"),
		"nested":  map[string]any{"count": json.Number("9007199254740993")},
	}
	data, err := opencodeMarshalJSON(value)
	if err != nil {
		t.Fatalf("opencodeMarshalJSON: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"timeout": 1500`, `"ratio": 1.50`, `"count": 9007199254740993`} {
		if !strings.Contains(text, want) {
			t.Fatalf("number literal %s lost: %s", want, text)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- test fixture
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string, content string) {
	t.Helper()
	writeTestFile(t, path, content)
}

func quoteJSON(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}
