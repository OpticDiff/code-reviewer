package repair

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"testing/quick"
)

func TestProperty_RepairJSON_Idempotence(t *testing.T) {
	// Property: Repairing an already repaired string is idempotent.
	// RepairJSON(RepairJSON(s)) == RepairJSON(s).
	property := func(s string) bool {
		r1, _ := RepairJSON(s)
		r2, _ := RepairJSON(r1)
		return r1 == r2
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_RepairJSON_Idempotence failed: %v", err)
	}
}

func TestProperty_RepairJSON_ValidJSONPreserved(t *testing.T) {
	// Property: Any valid JSON object remains valid JSON after RepairJSON,
	// and unmarshals to the exact same Go data.
	type samplePayload struct {
		ID      int               `json:"id"`
		Name    string            `json:"name"`
		Tags    []string          `json:"tags"`
		Meta    map[string]string `json:"meta"`
		Enabled bool              `json:"enabled"`
	}

	property := func(id int, name string, tags []string, keys, vals []string, enabled bool) bool {
		meta := make(map[string]string)
		for i := 0; i < len(keys) && i < len(vals); i++ {
			if keys[i] != "" {
				meta[keys[i]] = vals[i]
			}
		}
		original := samplePayload{
			ID:      id,
			Name:    name,
			Tags:    tags,
			Meta:    meta,
			Enabled: enabled,
		}
		raw, err := json.Marshal(original)
		if err != nil {
			return true // skip unmarshalable inputs if any
		}

		repaired, _ := RepairJSON(string(raw))
		var parsed samplePayload
		if err := json.Unmarshal([]byte(repaired), &parsed); err != nil {
			return false
		}

		return reflect.DeepEqual(original, parsed)
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_RepairJSON_ValidJSONPreserved failed: %v", err)
	}
}

func TestProperty_RepairJSON_MarkdownFencesStripped(t *testing.T) {
	// Property: Wrapping valid JSON in markdown code blocks ```json ... ```
	// is cleanly stripped and produces the same repaired content.
	property := func(key, val string) bool {
		if key == "" {
			key = "k"
		}
		rawObj, err := json.Marshal(map[string]string{key: val})
		if err != nil {
			return true
		}
		inner := string(rawObj)
		fenced := fmt.Sprintf("```json\n%s\n```", inner)

		repairedFenced, madeFencedRepairs := RepairJSON(fenced)
		repairedInner, _ := RepairJSON(inner)

		if !madeFencedRepairs {
			return false
		}
		return repairedFenced == repairedInner
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_RepairJSON_MarkdownFencesStripped failed: %v", err)
	}
}

func TestProperty_RepairJSON_DoubleEncodedUnwrapped(t *testing.T) {
	// Property: A valid JSON string that was double-encoded is unwrapped.
	property := func(msg string) bool {
		innerObj := map[string]string{"message": msg}
		innerJSON, err := json.Marshal(innerObj)
		if err != nil {
			return true
		}
		doubleEncoded, err := json.Marshal(string(innerJSON))
		if err != nil {
			return true
		}

		repaired, changed := RepairJSON(string(doubleEncoded))
		if !changed {
			return false
		}

		var parsed map[string]string
		if err := json.Unmarshal([]byte(repaired), &parsed); err != nil {
			return false
		}
		return parsed["message"] == msg
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("Property TestProperty_RepairJSON_DoubleEncodedUnwrapped failed: %v", err)
	}
}

func TestProperty_RepairedAcceptable_Monotonicity(t *testing.T) {
	// Property: RepairedAcceptable rejects repairs that drop more than half the bytes
	// or drop object delimiters.
	property := func(origLen uint16, repLen uint16) bool {
		ol := int(origLen)
		rl := int(repLen)
		if ol == 0 {
			return true
		}
		sOrig := string(make([]byte, ol))
		sRep := string(make([]byte, rl))

		acc := RepairedAcceptable(sOrig, sRep)
		if rl < ol/2 && acc {
			return false // must reject truncation > 50%
		}
		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("Property TestProperty_RepairedAcceptable_Monotonicity failed: %v", err)
	}
}
