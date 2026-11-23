package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"google.golang.org/genai"
)

// jsonFields returns the JSON field names of a struct type.
func jsonFields(t *testing.T, typ reflect.Type) map[string]bool {
	t.Helper()
	fields := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			t.Fatalf("%s.%s has no JSON name", typ.Name(), typ.Field(i).Name)
		}
		fields[name] = true
	}
	return fields
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestReviewResponseSchema_MatchesFinding fails when the schema and the
// Finding struct drift apart: a schema property Finding does not read would be
// dropped, and a Finding field neither in the schema nor in the list of fields
// the reviewer sets would be a field the model can never fill.
func TestReviewResponseSchema_MatchesFinding(t *testing.T) {
	schema := ReviewResponseSchema()

	// Top level: the fields of ReviewResult the model supplies.
	resultFields := jsonFields(t, reflect.TypeOf(ReviewResult{}))
	modelSupplied := map[string]bool{"summary": true, "findings": true}
	for name := range schema.Properties {
		if !resultFields[name] {
			t.Errorf("schema property %q is not a ReviewResult field", name)
		}
	}
	for name := range resultFields {
		_, inSchema := schema.Properties[name]
		if modelSupplied[name] != inSchema {
			t.Errorf("ReviewResult field %q: in schema = %v, want %v (usage and truncated are set by the tool)", name, inSchema, modelSupplied[name])
		}
	}
	if !reflect.DeepEqual(schema.Required, []string{"summary", "findings"}) {
		t.Errorf("top-level required = %v", schema.Required)
	}

	// Findings.
	item := schema.Properties["findings"].Items
	findingFields := jsonFields(t, reflect.TypeOf(Finding{}))
	for name := range item.Properties {
		if !findingFields[name] {
			t.Errorf("schema property %q is not a Finding field", name)
		}
		if _, set := reviewerSetFindingFields[name]; set {
			t.Errorf("schema property %q is filled in by the reviewer and must not be requested", name)
		}
	}
	for name := range findingFields {
		_, inSchema := item.Properties[name]
		_, reviewerSet := reviewerSetFindingFields[name]
		if inSchema == reviewerSet {
			t.Errorf("Finding field %q: in schema = %v, reviewer-set = %v; it must be exactly one", name, inSchema, reviewerSet)
		}
	}
	for name := range reviewerSetFindingFields {
		if !findingFields[name] {
			t.Errorf("reviewerSetFindingFields lists %q, which is not a Finding field", name)
		}
	}
	for _, name := range item.Required {
		if _, ok := item.Properties[name]; !ok {
			t.Errorf("required field %q is not a property", name)
		}
	}
	if len(item.PropertyOrdering) != len(item.Properties) {
		t.Errorf("property ordering %v does not list every property %v", item.PropertyOrdering, keys(item.Properties))
	}
	for _, name := range item.PropertyOrdering {
		if _, ok := item.Properties[name]; !ok {
			t.Errorf("property ordering lists unknown %q", name)
		}
	}
}

// TestReviewResponseSchema_MatchesPrompt checks the schema against the output
// format the built-in prompt shows the model.
func TestReviewResponseSchema_MatchesPrompt(t *testing.T) {
	_, afterHeading, ok := strings.Cut(basePrompt, "## OUTPUT FORMAT")
	if !ok {
		t.Fatal("prompt has no OUTPUT FORMAT section")
	}
	start := strings.Index(afterHeading, "{")
	end := strings.Index(afterHeading, "\n\nIf no issues are found")
	if start < 0 || end < start {
		t.Fatal("cannot find the example object in the OUTPUT FORMAT section")
	}
	var example struct {
		Summary  string           `json:"summary"`
		Findings []map[string]any `json:"findings"`
	}
	if err := json.Unmarshal([]byte(afterHeading[start:end]), &example); err != nil {
		t.Fatalf("example object does not parse: %v", err)
	}
	if len(example.Findings) != 1 {
		t.Fatalf("example has %d findings, want 1", len(example.Findings))
	}

	item := ReviewResponseSchema().Properties["findings"].Items
	for name := range example.Findings[0] {
		if _, ok := item.Properties[name]; !ok {
			t.Errorf("prompt example field %q is missing from the schema", name)
		}
	}
	for _, name := range item.Required {
		if _, ok := example.Findings[0][name]; !ok {
			t.Errorf("required field %q does not appear in the prompt example", name)
		}
	}
	// Optional in the schema means optional in the prompt.
	required := map[string]bool{}
	for _, name := range item.Required {
		required[name] = true
	}
	for _, name := range []string{"line", "old_line", "end_line", "rule_name", "suggestion"} {
		if required[name] {
			t.Errorf("%q must stay optional: the prompt marks it optional or exclusive", name)
		}
	}

	// Every severity the prompt defines is allowed, and nothing else.
	var inPrompt []string
	for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
		if strings.Contains(basePrompt, "* "+sev+":") {
			inPrompt = append(inPrompt, sev)
		}
	}
	if !reflect.DeepEqual(inPrompt, ReviewSeverities) || !reflect.DeepEqual(item.Properties["severity"].Enum, ReviewSeverities) {
		t.Errorf("severity enum = %v, prompt defines %v", item.Properties["severity"].Enum, inPrompt)
	}
	if item.Properties["category"].Enum != nil {
		t.Error("category must not be an enum: a rule may fix a category the prompt does not list")
	}
}

func TestParseReviewJSON_SchemaShapedAndFreeText(t *testing.T) {
	schemaShaped := `{"summary":"s","findings":[{"file":"a.go","line":3,"severity":"HIGH","category":"bug","title":"t","body":"b"},` +
		`{"file":"b.go","old_line":7,"severity":"LOW","category":"compliance","rule_name":"r","title":"t","body":"b","suggestion":"x"}]}`
	freeText := "Here is the review:\n```json\n" + schemaShaped + "\n```\nThanks."

	for name, text := range map[string]string{"schema-shaped": schemaShaped, "free text": freeText} {
		t.Run(name, func(t *testing.T) {
			res, err := parseReviewJSON(text)
			if err != nil {
				t.Fatalf("parseReviewJSON: %v", err)
			}
			if res.Summary != "s" || len(res.Findings) != 2 {
				t.Fatalf("result = %+v", res)
			}
			if f := res.Findings[1]; f.OldLine != 7 || f.Line != 0 || f.Category != "compliance" || f.RuleName != "r" {
				t.Errorf("second finding = %+v", f)
			}
		})
	}
}

// requestRecorder serves a canned review and keeps the request bodies.
func requestRecorder(t *testing.T, response string) (*Provider, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		out := map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": response}}}}}}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(server.Close)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI, APIKey: "k", HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Provider{client: client, modelName: "some-model"}, &bodies
}

func generationConfig(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	cfg, _ := body["generationConfig"].(map[string]any)
	if cfg == nil {
		t.Fatalf("request has no generationConfig: %v", body)
	}
	return cfg
}

func TestProvider_ReviewRequestSchema(t *testing.T) {
	const reply = `{"summary":"ok","findings":[]}`
	t.Run("on: the review call carries the schema and the JSON MIME type", func(t *testing.T) {
		p, bodies := requestRecorder(t, reply)
		p.EnableResponseSchema()
		if _, err := p.Review(context.Background(), "sys", "user"); err != nil {
			t.Fatal(err)
		}
		cfg := generationConfig(t, (*bodies)[0])
		if cfg["responseMimeType"] != "application/json" {
			t.Errorf("responseMimeType = %v", cfg["responseMimeType"])
		}
		schema, _ := cfg["responseSchema"].(map[string]any)
		if schema == nil {
			t.Fatalf("no responseSchema in %v", cfg)
		}
		wire, _ := json.Marshal(schema)
		var got genai.Schema
		if err := json.Unmarshal(wire, &got); err != nil {
			t.Fatal(err)
		}
		want := ReviewResponseSchema()
		wantWire, _ := json.Marshal(want)
		var wantRoundTrip genai.Schema
		_ = json.Unmarshal(wantWire, &wantRoundTrip)
		if !reflect.DeepEqual(got, wantRoundTrip) {
			t.Errorf("schema on the wire differs from ReviewResponseSchema:\n got %s\nwant %s", wire, wantWire)
		}
		if !strings.Contains(string(wire), `"CRITICAL"`) {
			t.Errorf("severity enum missing on the wire: %s", wire)
		}
	})

	t.Run("on: the main call cannot hit a function-call failure", func(t *testing.T) {
		// MALFORMED_FUNCTION_CALL needs tools or a tool config in the request;
		// the review call sets neither.
		p, bodies := requestRecorder(t, reply)
		p.EnableResponseSchema()
		if _, err := p.Review(context.Background(), "sys", "user"); err != nil {
			t.Fatal(err)
		}
		body := (*bodies)[0]
		for _, field := range []string{"tools", "toolConfig"} {
			if _, ok := body[field]; ok {
				t.Errorf("request carries %s", field)
			}
			if _, ok := generationConfig(t, body)[field]; ok {
				t.Errorf("generationConfig carries %s", field)
			}
		}
	})

	t.Run("off: no schema, and a non-Gemini name keeps no MIME type", func(t *testing.T) {
		p, bodies := requestRecorder(t, reply)
		if _, err := p.Review(context.Background(), "sys", "user"); err != nil {
			t.Fatal(err)
		}
		cfg := generationConfig(t, (*bodies)[0])
		if _, ok := cfg["responseSchema"]; ok {
			t.Errorf("responseSchema sent while off: %v", cfg["responseSchema"])
		}
		if _, ok := cfg["responseMimeType"]; ok {
			t.Errorf("responseMimeType sent while off: %v", cfg["responseMimeType"])
		}
	})

	t.Run("on: summary and explain calls never carry it", func(t *testing.T) {
		p, bodies := requestRecorder(t, `{"intent":"x"}`)
		p.EnableResponseSchema()
		_, _ = p.Summarize(context.Background(), "sys", "user") // Only the request is under test.
		_, _, _ = p.Explain(context.Background(), "sys", "user")
		if len(*bodies) != 2 {
			t.Fatalf("got %d requests, want 2", len(*bodies))
		}
		for i, body := range *bodies {
			if _, ok := generationConfig(t, body)["responseSchema"]; ok {
				t.Errorf("request %d carries the review schema", i)
			}
		}
	})

	t.Run("on: a schema-shaped reply and a fenced reply both parse", func(t *testing.T) {
		for _, reply := range []string{
			`{"summary":"s","findings":[{"file":"a.go","line":1,"severity":"LOW","category":"style","title":"t","body":"b"}]}`,
			"```json\n{\"summary\":\"s\",\"findings\":[]}\n```",
		} {
			p, _ := requestRecorder(t, reply)
			p.EnableResponseSchema()
			if _, err := p.Review(context.Background(), "sys", "user"); err != nil {
				t.Errorf("Review(%q): %v", reply, err)
			}
		}
	})
}
