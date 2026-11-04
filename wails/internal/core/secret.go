package core

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
