package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseYAML parses text into a yaml.Node tree. An empty document or a null
// root returns (nil, nil).
func ParseYAML(text string) (*yaml.Node, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	return documentRoot(&doc), nil
}

// documentRoot unwraps a DocumentNode to its root content node.
func documentRoot(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		return node.Content[0]
	}
	return node
}

// IsMapping reports whether node is a mapping.
func IsMapping(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.MappingNode
}

// IsSequence reports whether node is a sequence.
func IsSequence(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.SequenceNode
}

// MappingGet returns the value node for key, or nil.
func MappingGet(node *yaml.Node, key string) *yaml.Node {
	if !IsMapping(node) {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// MappingGetMap returns the mapping child for key.
func MappingGetMap(node *yaml.Node, key string) *yaml.Node {
	child := MappingGet(node, key)
	if IsMapping(child) {
		return child
	}
	return nil
}

// MappingGetSeq returns the sequence child for key.
func MappingGetSeq(node *yaml.Node, key string) *yaml.Node {
	child := MappingGet(node, key)
	if IsSequence(child) {
		return child
	}
	return nil
}

// MappingGetString returns the string value for key.
func MappingGetString(node *yaml.Node, key string) string {
	child := MappingGet(node, key)
	if child != nil && child.Kind == yaml.ScalarNode && child.Tag == "!!str" {
		return child.Value
	}
	if child != nil && child.Kind == yaml.ScalarNode && child.Tag == "" {
		return child.Value
	}
	return ""
}

// MappingSet inserts or replaces key with the given value node, preserving
// the position of existing keys.
func MappingSet(node *yaml.Node, key string, value *yaml.Node) {
	if !IsMapping(node) {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1] = value
			return
		}
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		value,
	)
}

// MappingRemove deletes key (and its value) from the mapping.
func MappingRemove(node *yaml.Node, key string) {
	if !IsMapping(node) {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}

// StringScalar builds a string scalar node.
func StringScalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// AnyScalar builds a scalar node from a Go value via YAML round-tripping.
func AnyScalar(value any) (*yaml.Node, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	return documentRoot(&node), nil
}

// MarshalYAML renders a node tree back to YAML text.
func MarshalYAML(node *yaml.Node) (string, error) {
	if node == nil {
		return "", nil
	}
	data, err := yaml.Marshal(node)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// NodeToAny decodes a node tree into generic Go values
// (map[string]any / []any / scalars).
func NodeToAny(node *yaml.Node) (any, error) {
	if node == nil {
		return nil, nil
	}
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, err
	}
	return normalizeYAMLValue(value), nil
}

// AnyToNode wraps generic values back into a node tree.
func AnyToNode(value any) (*yaml.Node, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	return documentRoot(&node), nil
}

// NodeToJSON converts a YAML node tree into its JSON equivalent by decoding
// to generic values and re-encoding as JSON. Useful for feeding YAML config
// sections into JSON-tagged Go structs.
func NodeToJSON(node *yaml.Node) ([]byte, error) {
	value, err := NodeToAny(node)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// JSONToNode converts a JSON document (e.g. the marshaled output of a typed
// struct) into a YAML node tree with identical key names.
func JSONToNode(data []byte) (*yaml.Node, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return AnyToNode(jsonNormalize(value))
}

// jsonNormalize converts json.Number-based trees into plain values that
// yaml.Marshal can render (numbers, strings, bools, slices, maps).
func jsonNormalize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = jsonNormalize(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = jsonNormalize(item)
		}
		return out
	default:
		return value
	}
}

func normalizeYAMLValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeYAMLValue(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[fmt.Sprint(key)] = normalizeYAMLValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeYAMLValue(item)
		}
		return out
	default:
		return value
	}
}

// LeadingCommentBlock returns the leading blank/comment lines of a document
// (the template header comments), used to preserve them when a document with
// no entries is rewritten.
func LeadingCommentBlock(original string) string {
	var block strings.Builder
	for _, line := range strings.Split(original, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			block.WriteString(line)
			block.WriteByte('\n')
			continue
		}
		break
	}
	return block.String()
}

// SplitPatchDocument splits a patch document into its header (everything
// before the first column-0 sequence item, comments and blanks included) and
// one raw text block per top-level sequence item.
func SplitPatchDocument(raw string) (string, []string) {
	var header strings.Builder
	var blocks []string
	var current strings.Builder
	inEntries := false
	for _, line := range strings.Split(raw, "\n") {
		isItem := line == "-" || strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "-\t")
		if isItem {
			if inEntries && current.Len() > 0 {
				blocks = append(blocks, current.String())
				current.Reset()
			}
			inEntries = true
			current.WriteString(line)
			current.WriteByte('\n')
		} else if inEntries {
			current.WriteString(line)
			current.WriteByte('\n')
		} else {
			header.WriteString(line)
			header.WriteByte('\n')
		}
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return header.String(), blocks
}

// SerializePatchEntry renders a mapping node as a sequence item: the first
// line prefixed with "- ", continuation lines indented two spaces. Node
// comments are stripped first — the raw text block keeps them, and
// re-emitting them here would duplicate the header.
func SerializePatchEntry(entry *yaml.Node) (string, error) {
	stripYAMLComments(entry)
	body, err := MarshalYAML(entry)
	if err != nil {
		return "", err
	}
	body = strings.TrimRight(body, "\n")
	var out strings.Builder
	out.WriteString("- ")
	for i, line := range strings.Split(body, "\n") {
		if i > 0 {
			out.WriteByte('\n')
			out.WriteString("  ")
		}
		out.WriteString(line)
	}
	out.WriteByte('\n')
	return out.String(), nil
}

// stripYAMLComments recursively clears comment fields on a node tree.
func stripYAMLComments(node *yaml.Node) {
	if node == nil {
		return
	}
	node.HeadComment = ""
	node.LineComment = ""
	node.FootComment = ""
	for _, child := range node.Content {
		stripYAMLComments(child)
	}
}

// TrailingNewline ensures text ends with exactly one newline.
func TrailingNewline(text string) string {
	return strings.TrimRight(text, "\n") + "\n"
}

// BytesOf is a tiny helper for embedding strings in writes.
func BytesOf(text string) []byte { return []byte(text) }

// BufferString renders a bytes.Buffer (kept for symmetry with callers).
func BufferString(buf *bytes.Buffer) string { return buf.String() }
