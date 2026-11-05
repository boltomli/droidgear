package core

import (
	"encoding/json"
	"io"
	"strings"
)

// DecodeJSONObject parses one JSON document leniently: trailing data and
// non-object documents are rejected (serde_json::from_str parity), numbers
// stay json.Number so `as_i64` vs `as_f64` semantics can be reproduced.
//
// It started life as sessions.go's sessionJSONMap and moved here once a
// non-session consumer (service/legacy) needed the exact same semantics.
func DecodeJSONObject(raw string) (map[string]any, bool) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil || out == nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return out, true
}
