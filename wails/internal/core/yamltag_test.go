package core

import (
	"strings"
	"testing"
)

func TestYamlV3PreservesJsTags(t *testing.T) {
	text := "- id: demo\n  config:\n    mode: !js $state.tools.mode\n    other: !!js $foo.bar\n"
	root, err := ParseYAML(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	item := root.Content[0]
	config := MappingGet(item, "config")
	if config == nil || !IsMapping(config) {
		t.Fatalf("config not parsed: %#v", config)
	}
	tags := []string{}
	for i := 0; i+1 < len(config.Content); i += 2 {
		tags = append(tags, config.Content[i+1].Tag+"="+config.Content[i+1].Value)
	}
	joined := strings.Join(tags, ", ")
	t.Logf("tags: %s", joined)
	if !strings.Contains(joined, "js=$state.tools.mode") || !strings.Contains(joined, "!!js") {
		t.Errorf("tag not preserved: %s", joined)
	}
	// Round-trip render
	out, err := MarshalYAML(root)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	t.Logf("round-trip:\n%s", out)
}
