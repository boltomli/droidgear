package core

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// ExtraField is the struct field name used to carry unknown JSON fields.
//
// The Rust core preserves unknown fields through `#[serde(flatten)] extra`
// maps so configs written by newer tool versions survive a DroidGear
// read/modify/write cycle. Go mirrors this: every model struct carries an
// `Extra map[string]json.RawMessage` field (tagged `json:"-"`) and uses
// UnmarshalJSONWithExtra / MarshalJSONWithExtra.

// UnmarshalJSONWithExtra decodes data into v (which must be a pointer to a
// struct), then collects any top-level JSON keys that do not match a struct
// field into the Extra map located by ExtraFieldName.
func UnmarshalJSONWithExtra(data []byte, v any) error {
	type plain any
	if err := json.Unmarshal(data, plain(v)); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	extra := findExtraField(v)
	if extra == nil {
		return nil
	}

	for _, name := range jsonFieldNames(v) {
		delete(raw, name)
	}
	if len(raw) == 0 {
		*extra = nil
		return nil
	}
	*extra = raw
	return nil
}

// MarshalJSONWithExtra marshals v and merges the Extra map back into the
// output. Keys already produced by known fields win over extras.
//
// v MUST be a method-less shadow of the target type (e.g. `type alias
// DshProviderConfig` converted by value), otherwise json.Marshal would
// dispatch back into the type's own MarshalJSON and recurse forever.
func MarshalJSONWithExtra(v any, extra map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		return data, nil
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(data, &merged); err != nil {
		return nil, err
	}
	for key, value := range extra {
		if _, ok := merged[key]; !ok {
			merged[key] = value
		}
	}
	return json.Marshal(merged)
}

// ExtraFieldName is the name of the struct field holding unknown keys.
const ExtraFieldName = "Extra"

func findExtraField(v any) *map[string]json.RawMessage {
	value := reflect.ValueOf(v)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil
	}
	field := value.FieldByName(ExtraFieldName)
	if !field.IsValid() || !field.CanAddr() {
		return nil
	}
	addr := field.Addr()
	if addr.Type() != reflect.TypeOf((*map[string]json.RawMessage)(nil)) {
		return nil
	}
	return addr.Interface().(*map[string]json.RawMessage)
}

// jsonFieldNames lists the JSON object keys produced by the struct's fields.
func jsonFieldNames(v any) []string {
	value := reflect.ValueOf(v)
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil
	}
	typeOf := value.Type()
	var names []string
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		if field.Name == ExtraFieldName {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := field.Name
		if tag != "" {
			if idx := indexByte(tag, ','); idx >= 0 {
				tag = tag[:idx]
			}
			if tag != "" {
				name = tag
			}
		}
		names = append(names, name)
	}
	return names
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// ToMap converts a JSON-serializable value into a generic map (round-trip
// through JSON). Used to translate YAML node trees into typed structs.
func ToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("value is not an object: %w", err)
	}
	return out, nil
}

// FromMap converts a generic map back into v (pointer).
func FromMap(m map[string]any, v any) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
