package app

import "testing"

func TestOpenclawNormalizeJSON5(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"strict input unchanged",
			`{"a":1,"b":[1,2]}`,
			`{"a":1,"b":[1,2]}`,
		},
		{
			"line and block comments",
			"{\n  // leading comment\n  \"a\": 1, /* inline */\n  \"b\": 2\n}",
			"{\n  \n  \"a\": 1, \n  \"b\": 2\n}",
		},
		{
			"trailing commas in objects and arrays",
			`{"a":[1,2,], "b":{"c":3,},}`,
			`{"a":[1,2], "b":{"c":3}}`,
		},
		{
			"single-quoted strings",
			`{'key': 'value with "double" quotes'}`,
			`{"key": "value with \"double\" quotes"}`,
		},
		{
			"comment between trailing comma and closer",
			`{"a":1, // note
}`,
			`{"a":1 
}`,
		},
		{
			"commas and slashes inside strings preserved",
			`{"url":"https://x//y, z'"} `,
			`{"url":"https://x//y, z'"} `,
		},
		{
			"escaped quotes in single-quoted strings",
			`{'a': 'he said \'hi\''}`,
			`{"a": "he said 'hi'"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(openclawNormalizeJSON5([]byte(tt.in)))
			if got != tt.want {
				t.Fatalf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// End-to-end: a JSON5 config with comments/trailing commas parses through
// openclawParseDocument and survives a read → marshal → re-parse cycle
// without losing unknown keys.
func TestOpenclawParseDocumentJSON5RoundTrip(t *testing.T) {
	src := []byte(`{
  // OpenClaw allows comments
  "models": {
    "providers": {
      "deepseek": { "baseUrl": "https://api.deepseek.com", /* block */ },
    },
  },
  "agents": { "defaults": { "model": { "primary": "deepseek-chat", }, } },
  "unknownFutureKey": [1, 2, 3,],
}`)

	document, err := openclawParseDocument(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	object, ok := document.(*openclawObject)
	if !ok {
		t.Fatalf("expected ordered object, got %T", document)
	}
	if !object.has("unknownFutureKey") {
		t.Fatal("unknownFutureKey missing after JSON5 parse")
	}
	models := object.entryObject("models")
	if models == nil {
		t.Fatal("models is not an object")
	}
	providers := models.entryObject("providers")
	if providers == nil {
		t.Fatal("models.providers is not an object")
	}
	deepseek := providers.entryObject("deepseek")
	if deepseek == nil {
		t.Fatal("deepseek provider is not an object")
	}
	if !deepseek.has("baseUrl") {
		t.Fatal("baseUrl missing")
	}

	// Re-marshal (strict JSON) and parse again — keys must survive.
	marshalled, err := openclawMarshalPretty(object)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	reparsed, err := openclawParseDocument(marshalled)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	reObject, ok := reparsed.(*openclawObject)
	if !ok {
		t.Fatalf("expected ordered object on re-parse, got %T", reparsed)
	}
	if !reObject.has("unknownFutureKey") {
		t.Fatal("unknownFutureKey lost in round trip")
	}
}
