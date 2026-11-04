package core

import "regexp"

// RedactSecret masks a secret for display: the last four characters survive
// so operators can still tell two keys apart, the rest becomes stars. Used
// by `dg` (secrets are redacted unless --reveal) and by dry-run previews —
// never by the write path, which always stores the real value
// (docs/developer/wails-tui-cli-design.zh.md §5.3).
func RedactSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return "****"
	}
	return "****" + secret[len(secret)-4:]
}

// sensitiveKey matches `someSecretKey: value` / `someSecretKey = "value"`
// across the three formats the tool configs use (JSON, TOML, YAML). Only the
// key name is captured, so the rest of the document — including its
// formatting — survives byte for byte: a preview has to show what would
// really be written, only masked.
//
// Bare `token` is in the list on purpose: matching is unanchored, so it also
// catches `bearer_token` and `accessToken`, while `tokensCount: 5` does not
// match because a separator has to follow the key name.
var sensitiveKey = regexp.MustCompile(`(?i)((?:experimental_bearer_token|openai_api_key|api[_-]?key|access[_-]?token|refresh[_-]?token|token|oauth|password|passwd|secret|authorization)["']?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\n,}\]]+)`)

// RedactSensitiveText masks the value of every secret-looking key in a
// document. Format-agnostic on purpose: JSON/TOML/YAML all spell a pair as
// `key: value` or `key = "value"`.
func RedactSensitiveText(text string) string {
	return sensitiveKey.ReplaceAllString(text, `${1}"****"`)
}
