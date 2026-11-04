package codex

import "errors"

// errString keeps the historical error wording. The GUI localises these
// exact strings in src/wails/error-i18n.ts, so they must not drift when the
// domain moves out of internal/app.
func errString(message string) error {
	return errors.New(message)
}
