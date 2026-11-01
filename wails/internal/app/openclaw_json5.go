package app

// JSON5 tolerance for ~/.openclaw/openclaw.json. OpenClaw documents the
// config as JSON5 — comments, trailing commas and single-quoted strings are
// explicitly allowed — while encoding/json only accepts strict JSON. The
// normaliser below rewrites JSON5 input into strict JSON before decoding;
// strict input passes through byte-identical, and every write path emits
// strict JSON, which JSON5 accepts.

// openclawNormalizeJSON5 returns src rewritten as strict JSON:
// line/block comments removed (newlines preserved for error positions),
// single-quoted strings converted to double quotes, and trailing commas
// before ] or } dropped. Scanning is byte-wise: JSON structural bytes are
// ASCII, so UTF-8 multibyte sequences are always copied verbatim.
func openclawNormalizeJSON5(src []byte) []byte {
	return openclawStripTrailingCommas(openclawStripCommentsAndSingleQuotes(src))
}

// openclawStripCommentsAndSingleQuotes removes comments and rewrites
// single-quoted strings. Callers follow up with
// openclawStripTrailingCommas, so a comment sitting between a comma and
// its closer cannot hide a trailing comma.
func openclawStripCommentsAndSingleQuotes(src []byte) []byte {
	out := make([]byte, 0, len(src))
	var quote byte // 0 = outside a string, otherwise the active delimiter
	for i := 0; i < len(src); i++ {
		c := src[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(src) {
				next := src[i+1]
				switch {
				case quote == '\'' && next == '\'': // JSON5 \' inside '…'
					out = append(out, '\'')
				case quote == '\'' && next == '"': // keep " escaped
					out = append(out, '\\', '"')
				default:
					out = append(out, c, next)
				}
				i++
				continue
			}
			if c == quote {
				quote = 0
				out = append(out, '"') // '…' → "…" (JSON5 strings are identical otherwise)
				continue
			}
			if quote == '\'' && c == '"' {
				out = append(out, '\\', '"')
				continue
			}
			out = append(out, c)
			continue
		}
		switch {
		case c == '"' || c == '\'':
			quote = c
			if c == '\'' {
				out = append(out, '"')
			} else {
				out = append(out, c)
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				if src[i] == '\n' {
					out = append(out, '\n')
				}
				i++
			}
			i++ // skip '/' of the closing '*/' (loop iteration skips '*')
		default:
			out = append(out, c)
		}
	}
	return out
}

// openclawStripTrailingCommas drops commas whose next non-space byte is a
// value closer. Strings are tracked so commas inside them are untouched.
func openclawStripTrailingCommas(src []byte) []byte {
	out := make([]byte, 0, len(src))
	var quote byte
	for i := 0; i < len(src); i++ {
		c := src[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(src) {
				out = append(out, c, src[i+1])
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			out = append(out, c)
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			out = append(out, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			if j < len(src) && (src[j] == ']' || src[j] == '}') {
				continue // drop the trailing comma
			}
		}
		out = append(out, c)
	}
	return out
}
