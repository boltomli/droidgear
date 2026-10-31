package app

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMarshalDoesNotRecurse guards against the stack-overflow panic that
// crashed the GUI when the Pi page listed profiles: MarshalJSON dispatched
// back into json.Marshal on the *same* type, recursing forever. The fix
// marshals a method-less alias type instead.
func TestMarshalDoesNotRecurse(t *testing.T) {
	provider := PiProviderConfig{
		BaseURL: strPtr("https://api.example.com/v1"),
		Models: []PiModel{{
			ID:    "gpt-5.2",
			Input: []string{"text"},
		}},
		Extra: map[string]json.RawMessage{"vendorFlag": json.RawMessage(`true`)},
	}
	data, err := json.Marshal(provider)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "vendorFlag") {
		t.Errorf("extra field lost on marshal: %s", text)
	}
	if !strings.Contains(text, "baseUrl") {
		t.Errorf("known field missing: %s", text)
	}
}

// TestProviderRoundTripPreservesExtras verifies that unknown fields survive a
// read → write cycle on every tool's provider type.
func TestProviderRoundTripPreservesExtras(t *testing.T) {
	t.Run("pi provider", func(t *testing.T) {
		input := `{"baseUrl":"https://x/v1","api":"openai-completions","futureField":{"a":1},` +
			`"models":[{"id":"m1","input":["text"],"modelFuture":"keep"}]}`
		var provider PiProviderConfig
		if err := json.Unmarshal([]byte(input), &provider); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := provider.Extra["futureField"]; !ok {
			t.Fatalf("provider extra lost on unmarshal: %#v", provider.Extra)
		}
		out, err := json.Marshal(provider)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		text := string(out)
		for _, want := range []string{"futureField", "modelFuture", "baseUrl"} {
			if !strings.Contains(text, want) {
				t.Errorf("round-trip lost %q: %s", want, text)
			}
		}
	})

	t.Run("dsh provider", func(t *testing.T) {
		input := `{"displayName":"x","baseURL":"https://x/v1","dshFuture":7,` +
			`"models":[{"id":"m1","modelFuture":"keep"}]}`
		var provider DshProviderConfig
		if err := json.Unmarshal([]byte(input), &provider); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if provider.BaseURL == nil || *provider.BaseURL != "https://x/v1" {
			t.Fatalf("baseURL not parsed: %#v", provider.BaseURL)
		}
		out, err := json.Marshal(provider)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		text := string(out)
		for _, want := range []string{"dshFuture", "modelFuture", "baseURL"} {
			if !strings.Contains(text, want) {
				t.Errorf("round-trip lost %q: %s", want, text)
			}
		}
	})

	t.Run("omp provider", func(t *testing.T) {
		input := `{"baseUrl":"https://x/v1","api":"openai-completions","apiKey":"k","auth":"apiKey","ompFuture":"keep",` +
			`"models":[{"id":"m1","modelFuture":true}]}`
		var provider OmpCustomProvider
		if err := json.Unmarshal([]byte(input), &provider); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if err := validateOmpCustomProvider(provider); err != nil {
			t.Fatalf("validation: %v", err)
		}
		out, err := json.Marshal(provider)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		text := string(out)
		for _, want := range []string{"ompFuture", "modelFuture", "baseUrl"} {
			if !strings.Contains(text, want) {
				t.Errorf("round-trip lost %q: %s", want, text)
			}
		}
	})
}
