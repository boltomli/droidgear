// Package channelimport holds the pure helpers shared by the Pi and OMP
// channel-import flows: provider-id suggestion, base-URL normalisation and
// API-protocol inference.
//
// The rules used to live in the React dialogs
// (src/components/{pi,omp}/*ImportFromChannel*), which meant the GUI was the
// only consumer and `dg`/TUI could not import a channel. They are pure
// string/registry transformations, so they belong in the service layer where
// all three consumers can share them (AGENTS rule 9).
package channelimport

import (
	"regexp"
	"strconv"
	"strings"
)

// API protocol identifiers written to Pi `models.json` / OMP `models.yml`.
const (
	APIOpenAICompletions  = "openai-completions"
	APIOpenAIResponses    = "openai-responses"
	APIAnthropicMessages  = "anthropic-messages"
	APIGoogleGenerativeAI = "google-generative-ai"
)

// DefaultContextWindow / DefaultMaxTokens mirror the dialog defaults used
// when the vendored registry has no entry for a channel model.
const (
	DefaultContextWindow uint32 = 128000
	DefaultMaxTokens     uint32 = 16384
)

var (
	nonProviderID = regexp.MustCompile(`[^a-z0-9_-]+`)
	versionPath   = regexp.MustCompile(`/v\d`)
)

// SanitizeProviderID makes a YAML/JSON-key-safe provider id out of a display
// name. An empty result is possible and must be handled by the caller.
func SanitizeProviderID(raw string) string {
	id := nonProviderID.ReplaceAllString(strings.ToLower(raw), "-")
	return strings.Trim(id, "-")
}

// SuggestProviderID derives a non-conflicting provider id from the channel
// (and optional token) name. Mirrors the GUI dialogs' suggestion rule.
func SuggestProviderID(channelName string, tokenName string, existing []string) string {
	base := channelName
	if strings.TrimSpace(tokenName) != "" {
		base = channelName + "-" + tokenName
	}
	base = SanitizeProviderID(base)
	if base == "" {
		base = "imported-provider"
	}
	if !contains(existing, base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if !contains(existing, candidate) {
			return candidate
		}
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// NormalizeBaseURL appends suffix unless the trimmed URL already ends with
// it (mirrors the frontend normalizeBaseUrl helper).
func NormalizeBaseURL(baseURL string, suffix string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if suffix == "" || strings.HasSuffix(trimmed, suffix) {
		return trimmed
	}
	return trimmed + suffix
}

// EnsureOpenAICompatibleV1 appends /v1 to an OpenAI-compatible endpoint
// unless it is empty or already carries a version segment (/v1, /v1beta …).
func EnsureOpenAICompatibleV1(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" || versionPath.MatchString(trimmed) {
		return trimmed
	}
	return strings.TrimRight(trimmed, "/") + "/v1"
}

// NeedsOpenAICompatibleV1 reports whether an OpenAI Chat Completions endpoint
// behind this channel type needs the /v1 suffix. Generic compatible routes
// always do; sub-2-api channels serving Chat Completions do too.
func NeedsOpenAICompatibleV1(channelType string, usesChatCompletions bool) bool {
	return channelType == "sub-2-api" && usesChatCompletions
}

// InferAPI maps a channel platform to the wire protocol Pi / OMP should use.
// An untagged platform is treated as OpenAI Chat Completions; antigravity is
// decided per model.
func InferAPI(platform *string, modelID string) string {
	value := ""
	if platform != nil {
		value = strings.ToLower(strings.TrimSpace(*platform))
	}
	switch value {
	case "anthropic", "claude":
		return APIAnthropicMessages
	case "gemini":
		return APIGoogleGenerativeAI
	case "antigravity":
		if strings.HasPrefix(strings.ToLower(modelID), "claude-") {
			return APIAnthropicMessages
		}
		return APIOpenAICompletions
	default:
		return APIOpenAICompletions
	}
}

// NormalizeBaseURLForChannel is the shared Pi/OMP rule: Chat Completions
// endpoints behind sub-2-api channels and generic compatible routes get the
// /v1 suffix; OpenAI-platform routes are normalised to /v1; Anthropic,
// Gemini and antigravity keep their channel base URL.
func NormalizeBaseURLForChannel(channelType string, baseURL string, platform *string) string {
	api := InferAPI(platform, "")
	if !isPlatform(platform, "antigravity") && NeedsOpenAICompatibleV1(channelType, api == APIOpenAICompletions) {
		return EnsureOpenAICompatibleV1(baseURL)
	}
	if platform == nil || strings.TrimSpace(*platform) == "" || isPlatform(platform, "openai") {
		return NormalizeBaseURL(baseURL, "/v1")
	}
	return strings.TrimRight(baseURL, "/")
}

func isPlatform(platform *string, want string) bool {
	return platform != nil && strings.EqualFold(strings.TrimSpace(*platform), want)
}

// SanitizeModelIDs drops blank entries and de-duplicates while keeping the
// caller's order (selected models arrive from a UI checkbox set).
func SanitizeModelIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
