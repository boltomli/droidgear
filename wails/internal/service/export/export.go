// Package export owns the channel export templates and the export engine.
// Mirrors droidgear-core::channel_export (Rust): templates live in
// ~/.droidgear/export-templates.yaml; a run collects channels/tokens/models,
// applies filters and field selectors, then writes JSON/YAML/TOML.
//
// The GUI (Preferences → Export), `dg export run <template>` and the future
// Go TUI share this engine so the three never disagree about what a template
// produces (design doc §2).
package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"droidgear-wails/internal/core"
	"droidgear-wails/internal/service/channels"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Types (camelCase JSON, matching the specta-generated TS types)
// ---------------------------------------------------------------------------

// ChannelFilter mirrors channel_export::ChannelFilter.
type ChannelFilter struct {
	Types       []string `json:"types,omitempty" yaml:"types,omitempty"`
	EnabledOnly bool     `json:"enabledOnly,omitempty" yaml:"enabledOnly,omitempty"`
	IDs         []string `json:"ids,omitempty" yaml:"ids,omitempty"`
}

// TokenFilter mirrors channel_export::TokenFilter.
type TokenFilter struct {
	Status    *int     `json:"status" yaml:"status,omitempty"`
	Platforms []string `json:"platforms,omitempty" yaml:"platforms,omitempty"`
}

// ExportTemplate mirrors channel_export::ExportTemplate.
type ExportTemplate struct {
	Name                   string            `json:"name" yaml:"name"`
	Description            string            `json:"description,omitempty" yaml:"description,omitempty"`
	Channels               ChannelFilter     `json:"channels" yaml:"channels"`
	Tokens                 TokenFilter       `json:"tokens" yaml:"tokens"`
	FetchModels            bool              `json:"fetchModels" yaml:"fetchModels"`
	ModelProtocolOverrides map[string]string `json:"modelProtocolOverrides,omitempty" yaml:"modelProtocolOverrides,omitempty"`
	Fields                 map[string]string `json:"fields,omitempty" yaml:"fields,omitempty"`
	Format                 string            `json:"format" yaml:"format"`
	OutputStructure        string            `json:"outputStructure" yaml:"outputStructure"`
	OutputPath             string            `json:"outputPath" yaml:"outputPath"`
}

// ExportResult mirrors channel_export::ExportResult.
type ExportResult struct {
	Template      string   `json:"template"`
	ChannelsCount int      `json:"channelsCount"`
	TokensCount   int      `json:"tokensCount"`
	ModelsCount   int      `json:"modelsCount"`
	OutputPath    string   `json:"outputPath"`
	RecordCount   int      `json:"recordCount"`
	Warnings      []string `json:"warnings"`
}

type exportConfig struct {
	Templates []ExportTemplate `yaml:"templates"`
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func exportTemplatesPath() (string, error) {
	dir, err := core.DroidGearDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "export-templates.yaml"), nil
}

func loadExportTemplates() ([]ExportTemplate, error) {
	path, err := exportTemplatesPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- app config path
	if err != nil {
		if os.IsNotExist(err) {
			return []ExportTemplate{}, nil
		}
		return nil, fmt.Errorf("Failed to read export templates: %w", err)
	}
	var config exportConfig
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("Failed to parse export templates: %w", err)
	}
	if config.Templates == nil {
		config.Templates = []ExportTemplate{}
	}
	return config.Templates, nil
}

func saveExportTemplates(templates []ExportTemplate) error {
	dir, err := core.DroidGearDir()
	if err != nil {
		return err
	}
	if err := core.EnsureDir(dir); err != nil {
		return fmt.Errorf("Failed to create .droidgear directory: %w", err)
	}
	raw, err := yaml.Marshal(exportConfig{Templates: templates})
	if err != nil {
		return fmt.Errorf("Failed to serialize export templates: %w", err)
	}
	path, err := exportTemplatesPath()
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, raw)
}

// LoadTemplates returns all export templates.
func LoadTemplates(ctx context.Context) ([]ExportTemplate, error) {
	return loadExportTemplates()
}

// SaveTemplate creates or replaces a template by name.
func SaveTemplate(ctx context.Context, template ExportTemplate) error {
	if strings.TrimSpace(template.Name) == "" {
		return errors.New("Template name cannot be empty")
	}
	templates, err := loadExportTemplates()
	if err != nil {
		return err
	}
	replaced := false
	for i := range templates {
		if templates[i].Name == template.Name {
			templates[i] = template
			replaced = true
			break
		}
	}
	if !replaced {
		templates = append(templates, template)
	}
	return saveExportTemplates(templates)
}

// DeleteTemplate removes a template by name.
func DeleteTemplate(ctx context.Context, name string) error {
	templates, err := loadExportTemplates()
	if err != nil {
		return err
	}
	kept := make([]ExportTemplate, 0, len(templates))
	for _, t := range templates {
		if t.Name != name {
			kept = append(kept, t)
		}
	}
	if len(kept) == len(templates) {
		return fmt.Errorf("Template '%s' not found", name)
	}
	return saveExportTemplates(kept)
}

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

// channelTypeDebug mirrors Rust's `format!("{:?}")` on ChannelType, which the
// flat records use for the "channel.type" field.
func channelTypeDebug(t string) string {
	switch t {
	case "new-api":
		return "NewApi"
	case "sub-2-api":
		return "Sub2Api"
	case "cli-proxy-api":
		return "CliProxyApi"
	case "ollama":
		return "Ollama"
	case "general":
		return "General"
	case "deep-seek":
		return "DeepSeek"
	default:
		return t
	}
}

func inferProtocolFromModelID(modelID string) string {
	lower := strings.ToLower(modelID)
	switch {
	case strings.HasPrefix(lower, "claude-"):
		return "anthropic"
	case strings.HasPrefix(lower, "gpt-"), strings.HasPrefix(lower, "o1-"), strings.HasPrefix(lower, "o3-"):
		return "openai"
	case strings.HasPrefix(lower, "gemini-"):
		return "google-ai"
	case strings.HasPrefix(lower, "deepseek-"):
		return "openai"
	default:
		return "openai-compatible"
	}
}

func resolveProtocol(modelID string, overrides map[string]string) string {
	for pattern, protocol := range overrides {
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(modelID, strings.TrimSuffix(pattern, "*")) {
				return protocol
			}
		} else if pattern == modelID {
			return protocol
		}
	}
	return inferProtocolFromModelID(modelID)
}

// buildFlatRecord mirrors channel_export::build_flat_record: either all
// well-known fields (no selector) or a source→rename mapping.
func buildFlatRecord(channel channels.Channel, token channels.ChannelToken, model *channels.ModelInfo, protocol string, fields map[string]string, redact bool) map[string]any {
	all := map[string]any{
		"channel.id":           channel.ID,
		"channel.name":         channel.Name,
		"channel.type":         channelTypeDebug(channel.Type),
		"channel.baseUrl":      channel.BaseURL,
		"channel.enabled":      channel.Enabled,
		"token.name":           token.Name,
		"token.key":            token.Key,
		"token.status":         token.Status,
		"token.platform":       token.Platform,
		"token.groupName":      token.GroupName,
		"token.remainQuota":    token.RemainQuota,
		"token.usedQuota":      token.UsedQuota,
		"token.unlimitedQuota": token.UnlimitedQuota,
	}
	if model != nil {
		all["model.id"] = model.ID
		all["model.name"] = model.Name
		all["model.protocol"] = protocol
	}
	if redact {
		// Mask before the field selector runs: a template may rename
		// token.key to anything, so the source has to be redacted first.
		all["token.key"] = core.RedactSecret(token.Key)
	}

	if len(fields) == 0 {
		return all
	}
	record := map[string]any{}
	for source, output := range fields {
		if value, ok := all[source]; ok {
			record[output] = value
			continue
		}
		switch source {
		case "model.id":
			if model != nil {
				record[output] = model.ID
			} else {
				record[output] = nil
			}
		case "model.name":
			if model != nil && model.Name != nil {
				record[output] = *model.Name
			} else {
				record[output] = nil
			}
		case "model.protocol":
			record[output] = protocol
		}
	}
	return record
}

// resolveOutputPath expands ~ and {timestamp}. The parent directory is only
// created for a real run: a dry-run preview must not touch the filesystem
// beyond reading it.
func resolveOutputPath(raw string, createDir bool) (string, error) {
	expanded := raw
	if strings.HasPrefix(raw, "~") {
		home, err := core.HomeDir()
		if err != nil {
			return "", err
		}
		expanded = filepath.Join(home, strings.TrimLeft(raw[1:], `/\`))
	}
	expanded = strings.ReplaceAll(expanded, "{timestamp}", time.Now().Format("20060102-150405"))
	if parent := filepath.Dir(expanded); parent != "" && createDir {
		if err := core.EnsureDir(parent); err != nil {
			return "", fmt.Errorf("Failed to create output directory '%s': %w", parent, err)
		}
	}
	return expanded, nil
}

// ensureExtension forces the file extension to match the format.
func ensureExtension(path string, format string) string {
	expected := map[string]string{"json": "json", "yaml": "yaml", "toml": "toml"}[format]
	if expected == "" {
		expected = "json"
	}
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	if strings.ToLower(ext) != expected {
		return strings.TrimSuffix(path, filepath.Ext(path)) + "." + expected
	}
	return path
}

func writeAtomicText(path string, content string) error {
	return core.AtomicWrite(path, []byte(content))
}

// RunOptions controls one template run.
type RunOptions struct {
	// DryRun computes the output document and reports it in Changes without
	// touching the filesystem (CLI --dry-run).
	DryRun bool
	// Reveal keeps secrets readable in a DryRun preview; without it the
	// preview is redacted (design doc §5.3). The written file always carries
	// the real values — redaction only ever affects what is displayed.
	Reveal bool
}

// Change is one file a run would write (ChangeSet shape, design doc §3.3).
type Change struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	After    string `json:"after"`
	Redacted bool   `json:"redacted"`
}

// RunOutcome reports what a run did, or would do when DryRun is set.
type RunOutcome struct {
	Applied bool         `json:"applied"`
	Result  ExportResult `json:"result"`
	Changes []Change     `json:"changes,omitempty"`
}

// Run executes a template and writes the output file; with RunOptions.DryRun
// it computes the same document and returns it without writing.
func Run(ctx context.Context, name string, opts RunOptions) (RunOutcome, error) {
	templates, err := loadExportTemplates()
	if err != nil {
		return RunOutcome{}, err
	}
	var template *ExportTemplate
	for i := range templates {
		if templates[i].Name == name {
			template = &templates[i]
			break
		}
	}
	if template == nil {
		return RunOutcome{}, fmt.Errorf("Template '%s' not found", name)
	}

	// Dry-run previews are redacted unless the caller opted into --reveal;
	// the written file always keeps the real values (design doc §5.3).
	redact := opts.DryRun && !opts.Reveal

	warnings := []string{}
	records := []map[string]any{}
	tokenCount := 0
	modelCount := 0

	channelList, err := channels.Load(ctx)
	if err != nil {
		return RunOutcome{}, err
	}

	matched := 0
	for _, channel := range channelList {
		if template.Channels.EnabledOnly && !channel.Enabled {
			continue
		}
		if len(template.Channels.Types) > 0 && !containsString(template.Channels.Types, channel.Type) {
			continue
		}
		if len(template.Channels.IDs) > 0 && !containsString(template.Channels.IDs, channel.ID) {
			continue
		}
		matched++

		tokens, err := channels.TokensFor(ctx, channel)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Channel '%s': %v", channel.Name, err))
			continue
		}
		for _, token := range tokens {
			if template.Tokens.Status != nil && token.Status != int32(*template.Tokens.Status) { // #nosec G115 -- small enum status
				continue
			}
			if len(template.Tokens.Platforms) > 0 {
				platform := ""
				if token.Platform != nil {
					platform = *token.Platform
				}
				if !containsString(template.Tokens.Platforms, platform) {
					continue
				}
			}
			tokenCount++

			if !template.FetchModels {
				records = append(records, buildFlatRecord(channel, token, nil, "", template.Fields, redact))
				continue
			}
			models, err := channels.FetchModelsByApiKey(ctx, channel.BaseURL, token.Key, token.Platform)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Channel '%s', token '%s': %v", channel.Name, token.Name, err))
			}
			if len(models) == 0 {
				records = append(records, buildFlatRecord(channel, token, nil, "", template.Fields, redact))
				continue
			}
			for i := range models {
				protocol := resolveProtocol(models[i].ID, template.ModelProtocolOverrides)
				records = append(records, buildFlatRecord(channel, token, &models[i], protocol, template.Fields, redact))
				modelCount++
			}
		}
	}

	outputPath, err := resolveOutputPath(template.OutputPath, !opts.DryRun)
	if err != nil {
		return RunOutcome{}, err
	}
	outputPath = ensureExtension(outputPath, template.Format)

	var content string
	switch template.Format {
	case "yaml":
		raw, err := yaml.Marshal(records)
		if err != nil {
			return RunOutcome{}, fmt.Errorf("YAML serialization failed: %w", err)
		}
		content = string(raw)
	case "toml":
		raw, err := toml.Marshal(map[string]any{"records": records})
		if err != nil {
			return RunOutcome{}, fmt.Errorf("TOML serialization failed: %w", err)
		}
		content = string(raw)
	default:
		raw, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return RunOutcome{}, fmt.Errorf("JSON serialization failed: %w", err)
		}
		content = string(raw)
	}
	result := ExportResult{
		Template:      template.Name,
		ChannelsCount: matched,
		TokensCount:   tokenCount,
		ModelsCount:   modelCount,
		OutputPath:    outputPath,
		RecordCount:   len(records),
		Warnings:      warnings,
	}

	if opts.DryRun {
		kind := template.Format
		if kind == "" {
			kind = "json"
		}
		return RunOutcome{
			Applied: false,
			Result:  result,
			Changes: []Change{{
				Path:     outputPath,
				Kind:     kind,
				After:    content,
				Redacted: redact,
			}},
		}, nil
	}

	if err := writeAtomicText(outputPath, content); err != nil {
		return RunOutcome{}, err
	}
	return RunOutcome{Applied: true, Result: result}, nil
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
