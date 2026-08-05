package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTool_OmitsAnnotationsWhenAbsent(t *testing.T) {
	data, err := json.Marshal(Tool{Name: "x", Description: "d", InputSchema: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); strings.Contains(got, "annotations") {
		t.Errorf("annotations key must be omitted when nil, got %s", got)
	}
}

func TestTool_SerialisesAllHintsExplicitly(t *testing.T) {
	tool := Tool{
		Name:        "x",
		Description: "d",
		InputSchema: json.RawMessage(`{}`),
		Annotations: &ToolAnnotations{
			Title:           "Do X",
			ReadOnlyHint:    true,
			DestructiveHint: false,
			IdempotentHint:  true,
			OpenWorldHint:   false,
		},
	}
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}

	var round struct {
		Annotations map[string]any `json:"annotations"`
	}
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}

	// A false hint must still be present — an omitted destructiveHint
	// means "true" per the MCP spec.
	for _, key := range []string{"title", "readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
		if _, ok := round.Annotations[key]; !ok {
			t.Errorf("annotations.%s missing from %s", key, data)
		}
	}
	if round.Annotations["destructiveHint"] != false {
		t.Errorf("destructiveHint = %v, want false", round.Annotations["destructiveHint"])
	}
	if round.Annotations["readOnlyHint"] != true {
		t.Errorf("readOnlyHint = %v, want true", round.Annotations["readOnlyHint"])
	}
}
