package app

// OpenClaw subagents: read/save of `agents.list` inside ~/.openclaw/openclaw.json.
// Port of the SubAgents section of droidgear-core::openclaw (Rust).

import (
	"encoding/json"
	"fmt"

	"droidgear-wails/internal/core"
)

// ---------------------------------------------------------------------------
// Types (mirroring droidgear-core::openclaw)
// ---------------------------------------------------------------------------

// OpenClawSubAgentIdentity mirrors openclaw::OpenClawSubAgentIdentity.
type OpenClawSubAgentIdentity struct {
	Emoji *string `json:"emoji,omitempty"`
	Name  *string `json:"name,omitempty"`
}

// OpenClawSubAgentModel mirrors openclaw::OpenClawSubAgentModel.
type OpenClawSubAgentModel struct {
	Primary   *string   `json:"primary,omitempty"`
	Fallbacks *[]string `json:"fallbacks,omitempty"`
}

// OpenClawSubAgentTools mirrors openclaw::OpenClawSubAgentTools.
type OpenClawSubAgentTools struct {
	Profile *string `json:"profile,omitempty"`
}

// OpenClawSubAgentSubagentsConfig mirrors
// openclaw::OpenClawSubAgentSubagentsConfig (the main agent's allowAgents).
type OpenClawSubAgentSubagentsConfig struct {
	AllowAgents   *[]string `json:"allowAgents,omitempty"`
	MaxConcurrent *uint32   `json:"maxConcurrent,omitempty"`
}

// OpenClawSubAgent mirrors openclaw::OpenClawSubAgent.
type OpenClawSubAgent struct {
	ID        string                           `json:"id"`
	Name      *string                          `json:"name,omitempty"`
	Identity  *OpenClawSubAgentIdentity        `json:"identity,omitempty"`
	Model     *OpenClawSubAgentModel           `json:"model,omitempty"`
	Tools     *OpenClawSubAgentTools           `json:"tools,omitempty"`
	Workspace *string                          `json:"workspace,omitempty"`
	Subagents *OpenClawSubAgentSubagentsConfig `json:"subagents,omitempty"`
}

// ---------------------------------------------------------------------------
// Deep merge (mirrors Rust deep_merge)
// ---------------------------------------------------------------------------

// openclawDeepMerge merges overlay into base: objects merge recursively,
// every other value (including arrays) is replaced wholesale. The merged
// value is returned — it is `base` when both sides are objects.
func openclawDeepMerge(base any, overlay any) any {
	baseObject, baseIsObject := base.(*openclawObject)
	overlayObject, overlayIsObject := overlay.(*openclawObject)
	if !baseIsObject || !overlayIsObject {
		return overlay
	}
	for _, key := range overlayObject.keys {
		overlayValue := overlayObject.values[key]
		if baseValue, ok := baseObject.values[key]; ok {
			baseObject.set(key, openclawDeepMerge(baseValue, overlayValue))
		} else {
			baseObject.set(key, overlayValue)
		}
	}
	return baseObject
}

// ---------------------------------------------------------------------------
// Subagent decode / encode helpers
// ---------------------------------------------------------------------------

// openclawDecodeSubAgent parses one agents.list entry. Like serde it
// requires a string `id`; entries without one are skipped by the reader.
func openclawDecodeSubAgent(data []byte) (OpenClawSubAgent, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return OpenClawSubAgent{}, err
	}
	if err := openclawRequireJSONString(fields, "id"); err != nil {
		return OpenClawSubAgent{}, err
	}
	var agent OpenClawSubAgent
	if err := json.Unmarshal(data, &agent); err != nil {
		return OpenClawSubAgent{}, err
	}
	return agent, nil
}

// openclawSubAgentToValue serialises a subagent the way serde_json::to_value
// does (struct field order, `None` fields omitted) and re-reads it into the
// ordered tree so it can be deep-merged into the config.
func openclawSubAgentToValue(agent OpenClawSubAgent) (any, error) {
	data, err := json.Marshal(agent)
	if err != nil {
		return nil, fmt.Errorf("Failed to serialize subagent: %w", err)
	}
	value, err := openclawParseDocument(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to serialize subagent: %w", err)
	}
	return value, nil
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// ReadOpenclawSubagents reads agents.list from the OpenClaw config file.
func (a *App) ReadOpenclawSubagents() ([]OpenClawSubAgent, error) {
	document, err := openclawReadConfigRaw()
	if err != nil {
		return nil, err
	}
	subagents := []OpenClawSubAgent{}
	root, ok := document.(*openclawObject)
	if !ok {
		return subagents, nil
	}
	agents, ok := root.get("agents").(*openclawObject)
	if !ok {
		return subagents, nil
	}
	list, ok := agents.get("list").([]any)
	if !ok {
		return subagents, nil
	}
	for _, item := range list {
		data, err := openclawMarshalValue(item)
		if err != nil {
			continue
		}
		agent, err := openclawDecodeSubAgent(data)
		if err != nil {
			continue // Rust skips entries that do not deserialize
		}
		subagents = append(subagents, agent)
	}
	return subagents, nil
}

// SaveOpenclawSubagents writes agents.list back into the config file:
// each incoming agent is deep-merged over its existing entry (so unknown
// fields survive), a `main` entry carrying allowAgents is synthesised when
// the list does not contain one, and agents missing from the incoming list
// are dropped.
func (a *App) SaveOpenclawSubagents(subagents []OpenClawSubAgent) error {
	path, err := openclawConfigPath()
	if err != nil {
		return err
	}
	document, err := openclawReadConfigRaw()
	if err != nil {
		return err
	}

	// Index the existing agents.list by id; entries are consumed as they are
	// matched (mirrors HashMap::remove in the Rust core).
	existing := map[string]any{}
	if root, ok := document.(*openclawObject); ok {
		if agents, ok := root.get("agents").(*openclawObject); ok {
			if list, ok := agents.get("list").([]any); ok {
				for _, item := range list {
					object, ok := item.(*openclawObject)
					if !ok {
						continue
					}
					if id, ok := object.get("id").(string); ok {
						existing[id] = item
					}
				}
			}
		}
	}

	nonMainIDs := []string{}
	for _, agent := range subagents {
		if agent.ID != "main" {
			nonMainIDs = append(nonMainIDs, agent.ID)
		}
	}

	resultList := []any{}
	for _, agent := range subagents {
		newValue, err := openclawSubAgentToValue(agent)
		if err != nil {
			return err
		}
		if currentValue, ok := existing[agent.ID]; ok {
			delete(existing, agent.ID)
			resultList = append(resultList, openclawDeepMerge(currentValue, newValue))
			continue
		}
		resultList = append(resultList, newValue)
	}

	// Ensure a main entry exists that allows every listed subagent.
	if len(nonMainIDs) > 0 && !openclawHasSubAgent(subagents, "main") {
		allowAgents := make([]any, 0, len(nonMainIDs))
		for _, id := range nonMainIDs {
			allowAgents = append(allowAgents, id)
		}
		subagentsConfig := newOpenclawObject()
		subagentsConfig.set("allowAgents", allowAgents)
		overlay := newOpenclawObject()
		overlay.set("id", "main")
		overlay.set("subagents", subagentsConfig)

		var mainEntry any = overlay
		if currentMain, ok := existing["main"]; ok {
			delete(existing, "main")
			mainEntry = openclawDeepMerge(currentMain, overlay)
		}
		resultList = append([]any{mainEntry}, resultList...)
	}

	listObject := newOpenclawObject()
	listObject.set("list", resultList)
	agentsOverlay := newOpenclawObject()
	agentsOverlay.set("agents", listObject)
	merged := openclawDeepMerge(document, agentsOverlay)

	data, err := openclawMarshalPretty(merged)
	if err != nil {
		return fmt.Errorf("Failed to serialize config: %w", err)
	}
	return core.AtomicWrite(path, data)
}

func openclawHasSubAgent(subagents []OpenClawSubAgent, id string) bool {
	for _, agent := range subagents {
		if agent.ID == id {
			return true
		}
	}
	return false
}
