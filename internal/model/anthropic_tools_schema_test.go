package model

import (
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
Two shapes arrive here and only one is what this provider takes.

A tool's schema is sometimes written as the properties alone — the platform's
own tools are — and sometimes as the whole object schema, which is what the
other provider's wire format wants. Sent as properties, the second becomes a
property called "type" whose value is the word "object", and the request is
refused as invalid with nothing saying which tool did it.
*/
func TestPropertiesOf_takesTheFieldsOutOfAWholeObjectSchema(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"text": map[string]any{"type": "string", "maxLength": 8192},
	}
	whole := map[string]any{
		"type":                 "object",
		"properties":           fields,
		"required":             []string{"text"},
		"additionalProperties": false,
	}

	if got := propertiesOf(whole); !reflect.DeepEqual(got, fields) {
		t.Fatalf("propertiesOf(whole) = %v, want the fields", got)
	}
	if got := propertiesOf(fields); !reflect.DeepEqual(got, fields) {
		t.Fatalf("propertiesOf(fields) = %v, want them unchanged", got)
	}
	if got := propertiesOf(nil); got != nil {
		t.Fatalf("propertiesOf(nil) = %v", got)
	}
}

// Both providers build the object around the fields, so both read a schema the
// same way. This is the path a LiteLLM-hosted model takes, and the one where a
// whole-object schema cost an afternoon.
func TestChatTools_aWholeObjectSchema_isSentAsFields(t *testing.T) {
	t.Parallel()
	fields := map[string]any{"text": map[string]any{"type": "string"}}
	client := &OpenAICompatible{tools: staticSchema{
		id: "ticket", desc: "answer", schema: map[string]any{
			"type": "object", "properties": fields,
		},
	}}

	tools := client.chatTools([]domain.ToolID{"ticket"}, namesFor(engine.PlanInput{
		Tools: []domain.ToolID{"ticket"},
	}))
	if len(tools) < 1 {
		t.Fatal("no tools were built")
	}
	params := tools[0].Function.Parameters
	if !reflect.DeepEqual(params["properties"], fields) {
		t.Fatalf("properties = %v, want the fields", params["properties"])
	}
}

type staticSchema struct {
	id     domain.ToolID
	desc   string
	schema map[string]any
}

func (s staticSchema) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	if id != s.id {
		return "", "", nil, false
	}
	return string(id), s.desc, s.schema, true
}
