package app

import "testing"

func TestMenuLabelsFor(t *testing.T) {
	for _, lang := range []string{"zh", "zh-CN", "zh-Hans-CN", "ZH"} {
		labels := menuLabelsFor(lang)
		if labels.preferences != "偏好设置..." {
			t.Fatalf("menuLabelsFor(%q) should pick Chinese labels, got %q", lang, labels.preferences)
		}
	}
	for _, lang := range []string{"en", "en-US", "", "fr-FR"} {
		labels := menuLabelsFor(lang)
		if labels.preferences != "Preferences..." {
			t.Fatalf("menuLabelsFor(%q) should pick English labels, got %q", lang, labels.preferences)
		}
	}
}

// The label tables must cover the same keys as the `menu.*` locale entries
// (locales/en.json / zh.json) — spot-check every field is non-empty and
// the two sets are structurally identical.
func TestMenuLabelSetsAreComplete(t *testing.T) {
	en := appMenuLabels["en"]
	zh := appMenuLabels["zh"]
	pairs := []struct{ en, zh string }{
		{en.about, zh.about},
		{en.preferences, zh.preferences},
		{en.checkForUpdates, zh.checkForUpdates},
		{en.hide, zh.hide},
		{en.hideOthers, zh.hideOthers},
		{en.showAll, zh.showAll},
		{en.quit, zh.quit},
		{en.view, zh.view},
		{en.edit, zh.edit},
		{en.undo, zh.undo},
		{en.redo, zh.redo},
		{en.cut, zh.cut},
		{en.copy, zh.copy},
		{en.paste, zh.paste},
		{en.selectAll, zh.selectAll},
		{en.toggleLeftSidebar, zh.toggleLeftSidebar},
		{en.toggleRightSidebar, zh.toggleRightSidebar},
		{en.resetWindow, zh.resetWindow},
	}
	for i, pair := range pairs {
		if pair.en == "" || pair.zh == "" {
			t.Fatalf("label pair %d must be non-empty in both languages: %q / %q", i, pair.en, pair.zh)
		}
	}
}
