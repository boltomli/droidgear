package app

import (
	"os"
	"path/filepath"
	"testing"
)

// legacyWritePair creates `<home>/factoryDir/config.json` plus a
// settings.json with the given customModels value.
func legacyWritePair(t *testing.T, factoryDir, customModels string) {
	t.Helper()
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "config.json"), []byte(`{"legacy":true}`), 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	settings := `{"customModels":` + customModels + `}`
	if err := os.WriteFile(filepath.Join(factoryDir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
}

// TestCheckLegacyConfig covers the dialog precondition: the deprecated
// config.json must exist AND settings.json must carry non-empty
// customModels (mirrors factory_settings::check_legacy_config_for_home).
func TestCheckLegacyConfig(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	factoryDir := filepath.Join(home, ".factory")

	// Nothing on disk → false, no error.
	found, err := a.CheckLegacyConfig()
	if err != nil || found {
		t.Fatalf("empty home: found=%v err=%v", found, err)
	}

	// Legacy file present, customModels migrated (non-empty) → true.
	legacyWritePair(t, factoryDir, `[{"model":"custom:One"}]`)
	found, err = a.CheckLegacyConfig()
	if err != nil || !found {
		t.Fatalf("migrated models: found=%v err=%v", found, err)
	}

	// Empty customModels array → false.
	legacyWritePair(t, factoryDir, `[]`)
	found, err = a.CheckLegacyConfig()
	if err != nil || found {
		t.Fatalf("empty models: found=%v err=%v", found, err)
	}

	// customModels of the wrong shape → false.
	legacyWritePair(t, factoryDir, `"nope"`)
	found, err = a.CheckLegacyConfig()
	if err != nil || found {
		t.Fatalf("non-array models: found=%v err=%v", found, err)
	}

	// Legacy file gone → false even though settings still carry models.
	if err := os.Remove(filepath.Join(factoryDir, "config.json")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	legacyWritePair(t, factoryDir, `[{"model":"custom:One"}]`)
	if err := os.Remove(filepath.Join(factoryDir, "config.json")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	found, err = a.CheckLegacyConfig()
	if err != nil || found {
		t.Fatalf("no legacy file: found=%v err=%v", found, err)
	}
}

// TestDeleteLegacyConfig covers removing the deprecated file (idempotent)
// while leaving settings.json untouched.
func TestDeleteLegacyConfig(t *testing.T) {
	home := isolatedHome(t)
	a := &App{}
	factoryDir := filepath.Join(home, ".factory")

	// Nothing to delete → no error.
	if err := a.DeleteLegacyConfig(); err != nil {
		t.Fatalf("delete on empty home: %v", err)
	}

	legacyWritePair(t, factoryDir, `[{"model":"custom:One"}]`)
	if err := a.DeleteLegacyConfig(); err != nil {
		t.Fatalf("DeleteLegacyConfig: %v", err)
	}
	if _, err := os.Stat(filepath.Join(factoryDir, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("config.json should be gone (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(factoryDir, "settings.json")); err != nil {
		t.Fatalf("settings.json must survive: %v", err)
	}

	// Second delete is a no-op.
	if err := a.DeleteLegacyConfig(); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	found, err := a.CheckLegacyConfig()
	if err != nil || found {
		t.Fatalf("after delete: found=%v err=%v", found, err)
	}
}
