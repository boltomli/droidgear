package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Pure logic (mirrors the Rust unit tests in connectivity.rs)
// ---------------------------------------------------------------------------

func TestConnectivityTestConfigDefaults(t *testing.T) {
	config := connectivityDefaultTestConfig()
	if config.TimeoutSeconds != 10 {
		t.Errorf("TimeoutSeconds = %d, want 10", config.TimeoutSeconds)
	}
	if !config.RetryOnFailure {
		t.Error("RetryOnFailure = false, want true")
	}
	if config.MaxRetries != 2 {
		t.Errorf("MaxRetries = %d, want 2", config.MaxRetries)
	}
}

func TestConnectivityProviderStringConversion(t *testing.T) {
	cases := map[string]string{
		"anthropic":                   "anthropic",
		"openai":                      "openai",
		"generic-chat-completion-api": "generic",
		"something-else":              "generic",
	}
	for input, want := range cases {
		if got := connectivityProviderString(input); got != want {
			t.Errorf("connectivityProviderString(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestConnectivityModeValidation(t *testing.T) {
	if inference, err := connectivityModeIsInference("ping"); err != nil || inference {
		t.Errorf("ping → (%v, %v), want (false, nil)", inference, err)
	}
	if inference, err := connectivityModeIsInference("inference"); err != nil || !inference {
		t.Errorf("inference → (%v, %v), want (true, nil)", inference, err)
	}
	_, err := connectivityModeIsInference("websocket")
	if err == nil || err.Error() != "unknown variant `websocket`, expected `ping` or `inference`" {
		t.Errorf("invalid mode error = %v, want serde-style unknown variant message", err)
	}
}

func TestConnectivityPromptOrDefault(t *testing.T) {
	if got := connectivityPromptOrDefault(nil); got != "Hi" {
		t.Errorf("nil prompt → %q, want %q", got, "Hi")
	}
	if got := connectivityPromptOrDefault(strPtr("")); got != "" {
		t.Errorf("empty prompt → %q, want empty (Option::Some is preserved)", got)
	}
	if got := connectivityPromptOrDefault(strPtr("custom")); got != "custom" {
		t.Errorf("custom prompt → %q, want %q", got, "custom")
	}
}

// ---------------------------------------------------------------------------
// Ping probe (mirrors probe_endpoint)
// ---------------------------------------------------------------------------

func TestConnectivityProbeEndpointSuccess(t *testing.T) {
	t.Run("openai style bearer auth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
				t.Errorf("request = %s %s, want GET /v1/models", r.Method, r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		if err := connectivityProbeEndpoint("openai", server.URL, "test-key", 2*time.Second); err != nil {
			t.Fatalf("probe: %v", err)
		}
	})

	t.Run("anthropic style api key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("x-api-key"); got != "test-key" {
				t.Errorf("x-api-key = %q, want %q", got, "test-key")
			}
			if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
				t.Errorf("anthropic-version = %q, want %q", got, "2023-06-01")
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("Authorization = %q, want empty for anthropic", got)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		if err := connectivityProbeEndpoint("anthropic", server.URL, "test-key", 2*time.Second); err != nil {
			t.Fatalf("probe: %v", err)
		}
	})
}

func TestConnectivityProbeEndpointErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "status line includes reason phrase",
			status: http.StatusNotFound,
			body:   "nope",
			want:   "HTTP 404 Not Found: nope",
		},
		{
			name:   "body truncated at 500 bytes",
			status: http.StatusInternalServerError,
			body:   strings.Repeat("x", 600),
			want:   "HTTP 500 Internal Server Error: " + strings.Repeat("x", 500) + "...",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			err := connectivityProbeEndpoint("openai", server.URL, "test-key", 2*time.Second)
			if err == nil {
				t.Fatal("probe succeeded, want error")
			}
			if err.Error() != tc.want {
				t.Errorf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestConnectivityProbeEndpointTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := connectivityProbeEndpoint("openai", server.URL, "test-key", 50*time.Millisecond)
	if err == nil {
		t.Fatal("probe succeeded, want timeout error")
	}
	if err.Error() != "Request timed out" {
		t.Errorf("error = %q, want %q", err.Error(), "Request timed out")
	}
}

func TestConnectivityPingWithoutAPIKey(t *testing.T) {
	diagnostics := connectivityNewTester().testModel("openai", "http://127.0.0.1:1", "", "gpt-x")
	if diagnostics.Success {
		t.Error("Success = true, want false")
	}
	if diagnostics.Error == nil || *diagnostics.Error != "No API key configured" {
		t.Errorf("Error = %v, want %q", diagnostics.Error, "No API key configured")
	}
	if diagnostics.TestMode != connectivityModePing {
		t.Errorf("TestMode = %q, want %q", diagnostics.TestMode, connectivityModePing)
	}
	if diagnostics.Provider != "openai" || diagnostics.ModelID != "gpt-x" {
		t.Errorf("provider/model = %q/%q, want openai/gpt-x", diagnostics.Provider, diagnostics.ModelID)
	}
	if diagnostics.ResponseText != nil || diagnostics.PromptUsed != nil {
		t.Error("ping diagnostics must not carry responseText/promptUsed")
	}
}

// ---------------------------------------------------------------------------
// Inference (mirrors run_inference)
// ---------------------------------------------------------------------------

func TestConnectivityInferenceOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request = %s %s, want POST /v1/chat/completions", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		body, _ := io.ReadAll(r.Body)
		var payload connectivityInferencePayload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("request body: %v", err)
		}
		if payload.Model != "m1" || payload.MaxTokens != 32 {
			t.Errorf("payload model/maxTokens = %q/%d, want m1/32", payload.Model, payload.MaxTokens)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" || payload.Messages[0].Content != "Hello" {
			t.Errorf("payload messages = %+v, want a single user message %q", payload.Messages, "Hello")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"Hi there!"}}]}`))
	}))
	defer server.Close()

	diagnostics := connectivityNewTester().testModelInference("openai", server.URL, "test-key", "m1", "Hello")
	if !diagnostics.Success {
		t.Fatalf("inference failed: %v", diagnostics.Error)
	}
	if diagnostics.TestMode != connectivityModeInference {
		t.Errorf("TestMode = %q, want %q", diagnostics.TestMode, connectivityModeInference)
	}
	if diagnostics.ResponseText == nil || *diagnostics.ResponseText != "Hi there!" {
		t.Errorf("ResponseText = %v, want %q", diagnostics.ResponseText, "Hi there!")
	}
	if diagnostics.PromptUsed == nil || *diagnostics.PromptUsed != "Hello" {
		t.Errorf("PromptUsed = %v, want %q", diagnostics.PromptUsed, "Hello")
	}
	if diagnostics.Provider != "openai" || diagnostics.ModelID != "m1" {
		t.Errorf("provider/model = %q/%q, want openai/m1", diagnostics.Provider, diagnostics.ModelID)
	}
}

func TestConnectivityInferenceAnthropic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			t.Errorf("request = %s %s, want POST /v1/messages", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q, want %q", got, "test-key")
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q, want %q", got, "2023-06-01")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Claude says hi"}]}`))
	}))
	defer server.Close()

	diagnostics := connectivityNewTester().testModelInference("anthropic", server.URL, "test-key", "m1", "Hello")
	if !diagnostics.Success {
		t.Fatalf("inference failed: %v", diagnostics.Error)
	}
	if diagnostics.ResponseText == nil || *diagnostics.ResponseText != "Claude says hi" {
		t.Errorf("ResponseText = %v, want %q", diagnostics.ResponseText, "Claude says hi")
	}
	if diagnostics.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", diagnostics.Provider)
	}
}

func TestConnectivityInferenceErrors(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		}))
		defer server.Close()

		diagnostics := connectivityNewTester().testModelInference("openai", server.URL, "test-key", "m1", "Hi")
		want := "Inference HTTP 500 Internal Server Error: boom"
		if diagnostics.Success || diagnostics.Error == nil || *diagnostics.Error != want {
			t.Errorf("diagnostics = %+v, want failure with %q", diagnostics, want)
		}
		if diagnostics.ResponseText != nil {
			t.Errorf("ResponseText = %v, want nil on failure", diagnostics.ResponseText)
		}
		if diagnostics.PromptUsed == nil || *diagnostics.PromptUsed != "Hi" {
			t.Errorf("PromptUsed = %v, want %q even on failure", diagnostics.PromptUsed, "Hi")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not-json{"))
		}))
		defer server.Close()

		diagnostics := connectivityNewTester().testModelInference("openai", server.URL, "test-key", "m1", "Hi")
		if diagnostics.Success || diagnostics.Error == nil ||
			!strings.HasPrefix(*diagnostics.Error, "Failed to parse inference response:") {
			t.Errorf("diagnostics = %+v, want parse failure", diagnostics)
		}
	})

	t.Run("missing choices yields empty response text", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[]}`))
		}))
		defer server.Close()

		diagnostics := connectivityNewTester().testModelInference("openai", server.URL, "test-key", "m1", "Hi")
		if !diagnostics.Success {
			t.Fatalf("inference failed: %v", diagnostics.Error)
		}
		if diagnostics.ResponseText == nil || *diagnostics.ResponseText != "" {
			t.Errorf("ResponseText = %v, want empty string (not omitted)", diagnostics.ResponseText)
		}
	})

	t.Run("no api key", func(t *testing.T) {
		diagnostics := connectivityNewTester().testModelInference("openai", "http://127.0.0.1:1", "", "m1", "Hi")
		if diagnostics.Success || diagnostics.Error == nil || *diagnostics.Error != "No API key configured" {
			t.Errorf("diagnostics = %+v, want No API key configured", diagnostics)
		}
		if diagnostics.TestMode != connectivityModeInference {
			t.Errorf("TestMode = %q, want %q", diagnostics.TestMode, connectivityModeInference)
		}
		if diagnostics.PromptUsed == nil || *diagnostics.PromptUsed != "Hi" {
			t.Errorf("PromptUsed = %v, want %q", diagnostics.PromptUsed, "Hi")
		}
	})
}

func TestConnectivityTruncateResponseText(t *testing.T) {
	short := "Hello"
	if got := connectivityTruncateResponseText(short); got != short {
		t.Errorf("short text = %q, want unchanged", got)
	}
	long := strings.Repeat("a", 250)
	got := connectivityTruncateResponseText(long)
	if len(got) != 203 || !strings.HasSuffix(got, "…") {
		t.Errorf("long text length/suffix = %d/%q, want 203 bytes (200 + 3-byte ellipsis)", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// Command layer (mirrors src-tauri/src/commands/connectivity.rs)
// ---------------------------------------------------------------------------

// writeConnectivityTestSettings drops a Factory settings file with the given
// customModels array into the isolated home.
func writeConnectivityTestSettings(t *testing.T, home, settingsJSON string) {
	t.Helper()
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir factory home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settingsJSON), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

type connectivityProbeRecord struct {
	bearer           string
	xAPIKey          string
	anthropicVersion string
}
