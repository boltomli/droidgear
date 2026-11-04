package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"droidgear-wails/internal/service/channels"
)

func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"} {
		t.Setenv(key, home)
	}
	return home
}

// seedChannel writes one API-key channel so the engine has something to
// export without touching the network (type "general" + stored key yields a
// synthetic token).
func seedChannel(t *testing.T, id string, apiKey string) {
	t.Helper()
	ctx := context.Background()
	list := []channels.Channel{{
		ID: id, Name: "Chan " + id, Type: "general",
		BaseURL: "http://unused.test", Enabled: true, CreatedAt: 1,
	}}
	if err := channels.Save(ctx, list); err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	if err := channels.SaveApiKey(ctx, id, apiKey); err != nil {
		t.Fatalf("seed api key: %v", err)
	}
}

func TestTemplateCRUD(t *testing.T) {
	sandbox(t)
	ctx := context.Background()

	if templates, err := LoadTemplates(ctx); err != nil || len(templates) != 0 {
		t.Fatalf("fresh store = %v, %v; want empty", templates, err)
	}

	template := ExportTemplate{
		Name:       "flat-json",
		Format:     "json",
		OutputPath: "~/out.json",
		Channels:   ChannelFilter{EnabledOnly: true},
	}
	if err := SaveTemplate(ctx, template); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	if err := SaveTemplate(ctx, ExportTemplate{Name: "   "}); err == nil {
		t.Error("empty name must be rejected")
	}

	templates, err := LoadTemplates(ctx)
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	if len(templates) != 1 || templates[0].Name != "flat-json" {
		t.Fatalf("templates = %+v", templates)
	}
	if !templates[0].Channels.EnabledOnly {
		t.Error("channel filter did not round-trip through YAML")
	}

	// Saving the same name replaces instead of duplicating.
	replacement := template
	replacement.Format = "yaml"
	if err := SaveTemplate(ctx, replacement); err != nil {
		t.Fatalf("SaveTemplate replace: %v", err)
	}
	templates, err = LoadTemplates(ctx)
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	if len(templates) != 1 || templates[0].Format != "yaml" {
		t.Fatalf("after replace = %+v", templates)
	}

	if err := DeleteTemplate(ctx, "missing"); err == nil {
		t.Error("deleting a missing template must fail")
	}
	if err := DeleteTemplate(ctx, "flat-json"); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	templates, err = LoadTemplates(ctx)
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	if len(templates) != 0 {
		t.Fatalf("store should be empty, got %+v", templates)
	}
}

func TestRunUnknownTemplate(t *testing.T) {
	sandbox(t)
	if _, err := Run(context.Background(), "nope", RunOptions{}); err == nil ||
		!strings.Contains(err.Error(), "Template 'nope' not found") {
		t.Errorf("error = %v", err)
	}
}

func TestRunDryRunPreviewsWithoutWritingAndRedacts(t *testing.T) {
	home := sandbox(t)
	ctx := context.Background()
	seedChannel(t, "ch1", "sk-export-1234")

	outputPath := filepath.Join(home, "out", "records.json")
	if err := SaveTemplate(ctx, ExportTemplate{
		Name:       "t1",
		Format:     "json",
		OutputPath: outputPath,
	}); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	// --- dry run: nothing on disk, secrets masked -----------------------
	dry, err := Run(ctx, "t1", RunOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.Applied {
		t.Error("dry run must report applied=false")
	}
	if len(dry.Changes) != 1 {
		t.Fatalf("expected one change, got %+v", dry.Changes)
	}
	change := dry.Changes[0]
	if change.Path != outputPath || change.Kind != "json" {
		t.Errorf("change = %+v, want path %q kind json", change, outputPath)
	}
	if !change.Redacted {
		t.Error("preview must be marked redacted")
	}
	if strings.Contains(change.After, "sk-export-1234") {
		t.Error("dry-run preview leaked the API key")
	}
	if !strings.Contains(change.After, "****1234") {
		t.Errorf("preview should carry the masked key, got %s", change.After)
	}
	if dry.Result.RecordCount != 1 || dry.Result.ChannelsCount != 1 {
		t.Errorf("result counts = %+v", dry.Result)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Error("dry run must not create the output file (or its directory)")
	}
	if _, err := os.Stat(filepath.Dir(outputPath)); !os.IsNotExist(err) {
		t.Error("dry run must not create the output directory")
	}

	// --- dry run with --reveal: full key --------------------------------
	revealed, err := Run(ctx, "t1", RunOptions{DryRun: true, Reveal: true})
	if err != nil {
		t.Fatalf("dry run reveal: %v", err)
	}
	if revealed.Changes[0].Redacted {
		t.Error("--reveal preview must not be marked redacted")
	}
	if !strings.Contains(revealed.Changes[0].After, "sk-export-1234") {
		t.Error("--reveal preview should contain the real key")
	}

	// --- real run: file written with the real values --------------------
	applied, err := Run(ctx, "t1", RunOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !applied.Applied || len(applied.Changes) != 0 {
		t.Fatalf("applied outcome = %+v", applied)
	}
	raw, err := os.ReadFile(outputPath) // #nosec G304 -- temp fixture
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(raw), "sk-export-1234") {
		t.Error("written file must contain the real key — redaction is display-only")
	}
	if !strings.Contains(string(raw), "ch1") {
		t.Errorf("written file should carry the channel id: %s", raw)
	}
	if applied.Result.OutputPath != outputPath {
		t.Errorf("result output path = %q, want %q", applied.Result.OutputPath, outputPath)
	}
}
