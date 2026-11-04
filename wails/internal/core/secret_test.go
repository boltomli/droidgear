package core

import "testing"

func TestRedactSensitiveTextMasksEveryFormat(t *testing.T) {
	cases := map[string]string{
		// JSON (profile documents, models.json)
		`{"apiKey": "sk-secret-1234", "name": "x"}`:    `{"apiKey": "****", "name": "x"}`,
		`{"oauth": "oauth-blob", "base": "https://a"}`: `{"oauth": "****", "base": "https://a"}`,
		// TOML (codex config.toml)
		`api_key = "sk-1"`:                   `api_key = "****"`,
		`experimental_bearer_token = "sk-2"`: `experimental_bearer_token = "****"`,
		// YAML (hermes config.yaml, omp models.yml)
		`api_key: sk-3`:              `api_key: "****"`,
		`OPENAI_API_KEY: "sk-4"`:     `OPENAI_API_KEY: "****"`,
		`authorization: Bearer sk-5`: `authorization: "****"`,
	}
	for input, want := range cases {
		if got := RedactSensitiveText(input); got != want {
			t.Errorf("RedactSensitiveText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRedactSensitiveTextKeepsEverythingElse(t *testing.T) {
	// Non-secret keys, including ones that merely contain a secret-ish word,
	// must survive untouched — a preview that mangles the document is worse
	// than no preview.
	inputs := []string{
		`{"key": "codex", "path": "/srv/codex"}`,
		`tokensCount = 5`,
		`"apiKeyHelper": "/bin/true"`,
		`env_key = "DEEPSEEK_API_KEY"`,
	}
	for _, input := range inputs {
		if got := RedactSensitiveText(input); got != input {
			t.Errorf("RedactSensitiveText(%q) = %q, want unchanged", input, got)
		}
	}
}

func TestRedactSecretKeepsLastFour(t *testing.T) {
	if got := RedactSecret("sk-secret-1234"); got != "****1234" {
		t.Errorf("RedactSecret = %q", got)
	}
	if got := RedactSecret("ab"); got != "****" {
		t.Errorf("short secret = %q", got)
	}
	if got := RedactSecret(""); got != "" {
		t.Errorf("empty secret = %q", got)
	}
}
