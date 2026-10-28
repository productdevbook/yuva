package api

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestMCPOutputSchemasStayOpen(t *testing.T) {
	for _, tool := range mcpTools {
		raw, err := json.Marshal(tool.output)
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if at := closedObject(doc, "#"); at != "" {
			t.Errorf("%s: output schema closes an object at %s", tool.name, at)
		}
	}
}

func closedObject(v any, path string) string {
	switch v := v.(type) {
	case map[string]any:
		if ap, ok := v["additionalProperties"]; ok {
			if ap == false {
				return path
			}
			if m, ok := ap.(map[string]any); ok && len(m) == 1 && m["not"] != nil {
				return path
			}
		}
		for k, c := range v {
			if at := closedObject(c, path+"/"+k); at != "" {
				return at
			}
		}
	case []any:
		for i, c := range v {
			if at := closedObject(c, path+"/"+strconv.Itoa(i)); at != "" {
				return at
			}
		}
	}
	return ""
}
