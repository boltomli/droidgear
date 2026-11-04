package channels

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(key, home)
	}
	return home
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadWithoutFilesReturnsEmptyAndWritesNothing(t *testing.T) {
	home := sandbox(t)
	ctx := context.Background()

	list, err := Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no channels, got %d", len(list))
	}
	if list == nil {
		t.Error("Load must return an empty slice, not nil (JSON contract is [])")
	}
	if _, err := os.Stat(filepath.Join(home, ".droidgear", "channels.json")); !os.IsNotExist(err) {
		t.Error("reading must not create channels.json")
	}
}

func TestLoadMigratesFactoryChannelsOnce(t *testing.T) {
	home := sandbox(t)
	ctx := context.Background()

	writeFile(t, filepath.Join(home, ".factory", "settings.json"), `{
	  "customModels": [],
	  "channels": [{"id":"m1","name":"Migrated","type":"general","baseUrl":"http://example.test","enabled":true,"createdAt":1}]
	}`)

	list, err := Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(list) != 1 || list[0].ID != "m1" {
		t.Fatalf("migrated channels = %+v, want one entry with id m1", list)
	}

	// The migration must have persisted the list, so the next Load reads it.
	migratedPath := filepath.Join(home, ".droidgear", "channels.json")
	raw, err := os.ReadFile(migratedPath) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read migrated channels: %v", err)
	}
	var persisted []Channel
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("parse migrated channels: %v", err)
	}
	if len(persisted) != 1 || persisted[0].ID != "m1" {
		t.Errorf("channels.json = %+v, want the migrated list", persisted)
	}
}

func TestSaveChannelsRoundTrip(t *testing.T) {
	sandbox(t)
	ctx := context.Background()

	want := []Channel{
		{ID: "a", Name: "A", Type: "new-api", BaseURL: "http://a.test", Enabled: true, CreatedAt: 1},
		{ID: "b", Name: "B", Type: "general", BaseURL: "http://b.test", Enabled: false, CreatedAt: 2},
	}
	if err := Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].BaseURL != "http://b.test" {
		t.Errorf("round trip = %+v", got)
	}
}

func TestCredentialStorageRoundTrip(t *testing.T) {
	home := sandbox(t)
	ctx := context.Background()

	if got, err := ApiKey(ctx, "ch1"); err != nil || got != nil {
		t.Fatalf("ApiKey on empty store = %v, %v; want nil, nil", got, err)
	}

	if err := SaveApiKey(ctx, "ch1", "sk-topsecret"); err != nil {
		t.Fatalf("SaveApiKey: %v", err)
	}
	key, err := ApiKey(ctx, "ch1")
	if err != nil {
		t.Fatalf("ApiKey: %v", err)
	}
	if key == nil || *key != "sk-topsecret" {
		t.Fatalf("ApiKey = %v, want sk-topsecret", key)
	}
	if creds, err := Credentials(ctx, "ch1"); err != nil || creds != nil {
		t.Fatalf("Credentials for an api_key entry = %v, %v; want nil, nil", creds, err)
	}

	authPath := filepath.Join(home, ".droidgear", "auth", "ch1.json")
	raw, err := os.ReadFile(authPath) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("parse auth file: %v", err)
	}
	if stored["type"] != "api_key" || stored["api_key"] != "sk-topsecret" {
		t.Errorf("auth file = %v", stored)
	}

	if err := SaveCredentials(ctx, "ch1", "user", "pass"); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	creds, err := Credentials(ctx, "ch1")
	if err != nil {
		t.Fatalf("Credentials: %v", err)
	}
	if creds == nil || (*creds)[0] != "user" || (*creds)[1] != "pass" {
		t.Fatalf("Credentials = %v", creds)
	}
	if key, err := ApiKey(ctx, "ch1"); err != nil || key != nil {
		t.Fatalf("ApiKey after overwrite = %v, %v; want nil, nil", key, err)
	}

	if err := DeleteCredentials(ctx, "ch1"); err != nil {
		t.Fatalf("DeleteCredentials: %v", err)
	}
	if creds, err := Credentials(ctx, "ch1"); err != nil || creds != nil {
		t.Fatalf("Credentials after delete = %v, %v; want nil, nil", creds, err)
	}
	// Deleting a channel that never had auth is not an error.
	if err := DeleteCredentials(ctx, "never-existed"); err != nil {
		t.Fatalf("DeleteCredentials on missing file: %v", err)
	}
}

func TestTokensForUsesStoredApiKeyWithoutNetwork(t *testing.T) {
	sandbox(t)
	ctx := context.Background()

	channel := Channel{ID: "ch1", Name: "Chan", Type: "general", BaseURL: "http://unused.test"}
	if err := SaveApiKey(ctx, "ch1", "sk-abcdef123456"); err != nil {
		t.Fatalf("SaveApiKey: %v", err)
	}

	tokens, err := TokensFor(ctx, channel)
	if err != nil {
		t.Fatalf("TokensFor: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected one synthetic token, got %d", len(tokens))
	}
	if tokens[0].Key != "sk-abcdef123456" || tokens[0].Status != 1 || !tokens[0].UnlimitedQuota {
		t.Errorf("synthetic token = %+v", tokens[0])
	}
}

func TestTokensForReportsMissingAuth(t *testing.T) {
	sandbox(t)
	ctx := context.Background()

	if _, err := TokensFor(ctx, Channel{ID: "ch1", Name: "NoKey", Type: "general"}); err == nil ||
		!strings.Contains(err.Error(), "No API key for channel 'NoKey'") {
		t.Errorf("missing api key error = %v", err)
	}
	if _, err := TokensFor(ctx, Channel{ID: "ch2", Name: "NoCreds", Type: "new-api"}); err == nil ||
		!strings.Contains(err.Error(), "No credentials for channel 'NoCreds'") {
		t.Errorf("missing credentials error = %v", err)
	}
}

func TestFetchTokensMarksUpstreamFailuresWithoutChangingTheMessage(t *testing.T) {
	sandbox(t)
	ctx := context.Background()

	// Port 1 on loopback refuses immediately: no DNS, no external network.
	_, err := FetchTokens(ctx, "new-api", "http://127.0.0.1:1", "user", "pass")
	if err == nil {
		t.Fatal("expected a login failure")
	}
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("error should be marked ErrUpstream for exit code 4, got %v", err)
	}
	// The GUI shows this string verbatim (error-i18n), so the mark must not
	// leak into the text.
	if strings.Contains(err.Error(), ErrUpstream.Error()) {
		t.Errorf("sentinel text leaked into the user-facing message: %q", err.Error())
	}
}
