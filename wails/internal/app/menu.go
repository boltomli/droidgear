// Native application menu for the Wails shell. The Tauri build constructs
// the same menu from JS (src/lib/menu.ts) so react-i18next can supply the
// labels; here the frontend injects the localised labels through
// BuildAppMenu(lang), called at startup and whenever the language changes
// (src/wails/menu-bridge.ts). Items either act natively (About dialog,
// Hide, Quit) or emit `menu:*` events that the bridge translates into the
// same store actions as src/lib/menu.ts.
//
// "Check for Updates" is intentionally absent: the auto-updater is
// Tauri-only by design (see capabilities.isUpdaterSupported).
package app

import (
	"fmt"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// menuLabelSet mirrors the `menu.*` keys in locales/en.json / zh.json.
// Keep the two tables in sync when adding menu items.
type menuLabelSet struct {
	about              string
	preferences        string
	hide               string
	hideOthers         string
	showAll            string
	quit               string
	view               string
	edit               string
	undo               string
	redo               string
	cut                string
	copy               string
	paste              string
	selectAll          string
	toggleLeftSidebar  string
	toggleRightSidebar string
	resetWindow        string
}

var appMenuLabels = map[string]menuLabelSet{
	"en": {
		about:              "About DroidGear",
		preferences:        "Preferences...",
		hide:               "Hide DroidGear",
		hideOthers:         "Hide Others",
		showAll:            "Show All",
		quit:               "Quit DroidGear",
		view:               "View",
		edit:               "Edit",
		undo:               "Undo",
		redo:               "Redo",
		cut:                "Cut",
		copy:               "Copy",
		paste:              "Paste",
		selectAll:          "Select All",
		toggleLeftSidebar:  "Toggle Left Sidebar",
		toggleRightSidebar: "Toggle Right Sidebar",
		resetWindow:        "Reset Window",
	},
	"zh": {
		about:              "关于 DroidGear",
		preferences:        "偏好设置...",
		hide:               "隐藏 DroidGear",
		hideOthers:         "隐藏其他",
		showAll:            "显示全部",
		quit:               "退出 DroidGear",
		view:               "视图",
		edit:               "编辑",
		undo:               "撤销",
		redo:               "重做",
		cut:                "剪切",
		copy:               "复制",
		paste:              "粘贴",
		selectAll:          "全选",
		toggleLeftSidebar:  "切换左侧边栏",
		toggleRightSidebar: "切换右侧边栏",
		resetWindow:        "重置窗口",
	},
}

// menuLabelsFor picks the label set for a BCP-47-ish language tag.
func menuLabelsFor(lang string) menuLabelSet {
	if len(lang) >= 2 && (lang[0] == 'z' || lang[0] == 'Z') && (lang[1] == 'h' || lang[1] == 'H') {
		return appMenuLabels["zh"]
	}
	return appMenuLabels["en"]
}

// BuildAppMenu (re)builds the native application menu using labels for the
// given language ("zh", "zh-CN", … → Chinese; anything else → English).
// Mirrors the Tauri menu built by src/lib/menu.ts (minus the Tauri-only
// updater entry).
func (a *App) BuildAppMenu(lang string) error {
	app := application.Get()
	if app == nil {
		return errString("Cannot build menu: application is not running")
	}
	labels := menuLabelsFor(lang)
	menu := app.NewMenu()

	// App menu — appears as the app name on macOS, as "DroidGear" on
	// Windows/Linux (the window opts into the application menu).
	appMenu := menu.AddSubmenu("DroidGear")
	appMenu.Add(labels.about).OnClick(func(*application.Context) {
		app.Dialog.Info().
			SetTitle("DroidGear").
			SetMessage(fmt.Sprintf(
				"DroidGear\n\nVersion: %s\n\nBuilt with Wails v3 + React",
				appVersion,
			)).
			Show()
	})
	appMenu.AddSeparator()
	appMenu.Add(labels.preferences).SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) {
		app.Event.Emit("menu:open-preferences", map[string]any{})
	})
	appMenu.AddSeparator()
	appMenu.Add(labels.hide).SetAccelerator("CmdOrCtrl+h").OnClick(func(*application.Context) {
		app.Hide()
	})
	if runtime.GOOS == "darwin" {
		// Native macOS semantics (hide other apps / reveal all windows).
		appMenu.AddRole(application.HideOthers)
		appMenu.AddRole(application.ShowAll)
	} else {
		// Kept for structural parity with the Tauri menu; no cross-platform
		// equivalent of "hide other apps" exists.
		appMenu.Add(labels.hideOthers).SetEnabled(false)
		appMenu.Add(labels.showAll).SetEnabled(false)
	}
	appMenu.AddSeparator()
	appMenu.Add(labels.quit).SetAccelerator("CmdOrCtrl+q").OnClick(func(*application.Context) {
		app.Quit()
	})

	// Edit menu — role-based so Cut/Copy/Paste/Undo/Redo work natively in
	// the webview (required for ⌘C/⌘V on macOS); the roles' default labels
	// are overridden with the locale strings.
	menu.AddRole(application.EditMenu)
	setRoleLabel := func(role application.Role, text string) {
		if item := menu.FindByRole(role); item != nil {
			item.SetLabel(text)
		}
	}
	setRoleLabel(application.EditMenu, labels.edit)
	setRoleLabel(application.Undo, labels.undo)
	setRoleLabel(application.Redo, labels.redo)
	setRoleLabel(application.Cut, labels.cut)
	setRoleLabel(application.Copy, labels.copy)
	setRoleLabel(application.Paste, labels.paste)
	setRoleLabel(application.SelectAll, labels.selectAll)

	// View menu — actions operate on frontend state, so they are emitted
	// as events (see src/wails/menu-bridge.ts).
	viewMenu := menu.AddSubmenu(labels.view)
	viewMenu.Add(labels.toggleLeftSidebar).OnClick(func(*application.Context) {
		app.Event.Emit("menu:toggle-left-sidebar", map[string]any{})
	})
	viewMenu.Add(labels.toggleRightSidebar).OnClick(func(*application.Context) {
		app.Event.Emit("menu:toggle-right-sidebar", map[string]any{})
	})
	viewMenu.AddSeparator()
	viewMenu.Add(labels.resetWindow).SetAccelerator("CmdOrCtrl+Shift+0").OnClick(func(*application.Context) {
		app.Event.Emit("menu:reset-window", map[string]any{})
	})

	app.Menu.Set(menu)
	return nil
}
