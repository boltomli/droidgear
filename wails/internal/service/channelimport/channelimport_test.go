package channelimport

import "testing"

func ptr(value string) *string { return &value }

func TestSuggestProviderID(t *testing.T) {
	cases := []struct {
		name     string
		channel  string
		token    string
		existing []string
		want     string
	}{
		{"slug", "My Channel!", "", nil, "my-channel"},
		{"token suffix", "My Channel", "Tok 1", nil, "my-channel-tok-1"},
		{"conflict", "chan", "", []string{"chan"}, "chan-2"},
		{"deep conflict", "chan", "", []string{"chan", "chan-2", "chan-3"}, "chan-4"},
		{"empty", "!!!", "", nil, "imported-provider"},
	}
	for _, tc := range cases {
		if got := SuggestProviderID(tc.channel, tc.token, tc.existing); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestEnsureOpenAICompatibleV1(t *testing.T) {
	cases := map[string]string{
		"https://api.example.com":             "https://api.example.com/v1",
		"https://api.example.com/":            "https://api.example.com/v1",
		"https://api.example.com/v1":          "https://api.example.com/v1",
		"https://api.example.com/v1beta":      "https://api.example.com/v1beta",
		"https://api.example.com/antigravity": "https://api.example.com/antigravity/v1",
		"":                                    "",
	}
	for in, want := range cases {
		if got := EnsureOpenAICompatibleV1(in); got != want {
			t.Errorf("EnsureOpenAICompatibleV1(%q) = %q want %q", in, got, want)
		}
	}
}

func TestInferAPI(t *testing.T) {
	cases := []struct {
		platform *string
		model    string
		want     string
	}{
		{nil, "", APIOpenAICompletions},
		{ptr("openai"), "gpt-5", APIOpenAICompletions},
		{ptr("anthropic"), "claude-x", APIAnthropicMessages},
		{ptr("gemini"), "gemini-3", APIGoogleGenerativeAI},
		{ptr("antigravity"), "claude-sonnet-4-5", APIAnthropicMessages},
		{ptr("antigravity"), "gemini-3-pro", APIOpenAICompletions},
	}
	for _, tc := range cases {
		if got := InferAPI(tc.platform, tc.model); got != tc.want {
			t.Errorf("InferAPI(%v, %q) = %q want %q", tc.platform, tc.model, got, tc.want)
		}
	}
}

func TestNormalizeBaseURLForChannel(t *testing.T) {
	cases := []struct {
		channel  string
		baseURL  string
		platform *string
		want     string
	}{
		{"sub-2-api", "https://api.example.com", ptr("openai"), "https://api.example.com/v1"},
		{"sub-2-api", "https://api.example.com/v1", nil, "https://api.example.com/v1"},
		{"new-api", "https://api.example.com", ptr("openai"), "https://api.example.com/v1"},
		{"new-api", "https://api.example.com", ptr("anthropic"), "https://api.example.com"},
		{"new-api", "https://api.example.com/", ptr("gemini"), "https://api.example.com"},
		{"new-api", "https://api.example.com", ptr("antigravity"), "https://api.example.com"},
		{"general", "https://api.example.com/v1", nil, "https://api.example.com/v1"},
	}
	for _, tc := range cases {
		if got := NormalizeBaseURLForChannel(tc.channel, tc.baseURL, tc.platform); got != tc.want {
			t.Errorf("%s %q %v: got %q want %q", tc.channel, tc.baseURL, tc.platform, got, tc.want)
		}
	}
}
