package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"droidgear-wails/internal/testutil"
)

func seedTuiChannel(t *testing.T, home string) {
	t.Helper()
	writeTuiFile(t, home, ".droidgear/channels.json",
		`[{"id":"c1","name":"My Chan","type":"general","baseUrl":"https://api.example.com","enabled":true,"createdAt":0}]`)
	writeTuiFile(t, home, ".droidgear/auth/c1.json", `{"type":"api_key","api_key":"sk-test"}`)
}

func writeTuiFile(t *testing.T, home string, rel string, contents string) {
	t.Helper()
	path := filepath.Join(home, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil { // #nosec G306 -- test fixture
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestParseImportValue(t *testing.T) {
	cases := []struct {
		value   string
		channel string
		token   string
		models  []string
	}{
		{"c1", "c1", "", nil},
		{"c1 tok", "c1", "tok", nil},
		{"c1 tok m1,m2", "c1", "tok", []string{"m1", "m2"}},
		{"", "", "", nil},
	}
	for _, tc := range cases {
		channel, token, models := parseImportValue(tc.value)
		if channel != tc.channel || token != tc.token || len(models) != len(tc.models) {
			t.Errorf("parseImportValue(%q) = %q %q %v", tc.value, channel, token, models)
		}
	}
}

func TestPiProvidersSourceImportsFromChannel(t *testing.T) {
	home := testutil.IsolatedHome(t)
	seedTuiChannel(t, home)

	source := PiProvidersSource()
	actions := source.(InputActionSource).InputActions()
	if len(actions) != 1 || actions[0].Key != "i" {
		t.Fatalf("input actions = %+v", actions)
	}
	action := actions[0].Submit(context.Background(), ListRow{}, "c1 - claude-opus-5-5")
	if err := action.run(); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	path := filepath.Join(home, ".pi", "agent", "models.json")
	data, err := os.ReadFile(path) // #nosec G304 -- isolated test path
	if err != nil {
		t.Fatalf("read models.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	providers := root["providers"].(map[string]any)
	if len(providers) != 1 {
		t.Fatalf("providers = %v", providers)
	}
	rows, err := source.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(rows) != 1 || rows[0].ID == "" {
		t.Errorf("rows = %+v", rows)
	}
	detail, err := source.Detail(context.Background(), rows[0].ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if string(detail) == "" || containsSecret(detail) {
		t.Errorf("detail leaked the key: %s", detail)
	}
}

func containsSecret(raw []byte) bool {
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return true
	}
	value, _ := decoded["apiKey"].(string)
	return value == "sk-test"
}
