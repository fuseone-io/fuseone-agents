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

	if got, _ := propertiesOf(whole); !reflect.DeepEqual(got, fields) {
		t.Fatalf("propertiesOf(whole) = %v, want the fields", got)
	}
	if got, _ := propertiesOf(fields); !reflect.DeepEqual(got, fields) {
		t.Fatalf("propertiesOf(fields) = %v, want them unchanged", got)
	}
	if got, _ := propertiesOf(nil); got != nil {
		t.Fatalf("propertiesOf(nil) = %v", got)
	}
}

// A whole-object schema says which fields are mandatory beside the fields
// themselves. Taking only the fields made every one of them optional, and the
// model proposed calls the connector then refused as bad arguments. A schema
// decoded from JSON carries the list as []any, one written in Go as []string.
func TestPropertiesOf_wholeObjectSchema_keepsItsRequiredFields(t *testing.T) {
	t.Parallel()
	fields := map[string]any{"subscriptionId": map[string]any{"type": "string"}}
	cases := map[string]any{
		"written in Go":     []string{"subscriptionId"},
		"decoded from JSON": []any{"subscriptionId"},
	}
	for name, required := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, got := propertiesOf(map[string]any{
				"type": "object", "properties": fields, "required": required,
			})
			if !reflect.DeepEqual(got, []string{"subscriptionId"}) {
				t.Fatalf("required = %v, want [subscriptionId]", got)
			}
		})
	}
}

func TestToolParams_aWholeObjectSchema_sendsItsRequiredFields(t *testing.T) {
	t.Parallel()
	client := &Anthropic{tools: requiredSchema()}

	tools := client.toolParams([]domain.ToolID{"ticket"}, namesFor(engine.PlanInput{
		Tools: []domain.ToolID{"ticket"},
	}))
	if len(tools) < 1 || tools[0].OfTool == nil {
		t.Fatal("no tools were built")
	}
	got := tools[0].OfTool.InputSchema.Required
	if !reflect.DeepEqual(got, []string{"subscriptionId"}) {
		t.Fatalf("required = %v, want [subscriptionId]", got)
	}
}

func TestChatTools_aWholeObjectSchema_sendsItsRequiredFields(t *testing.T) {
	t.Parallel()
	client := &OpenAICompatible{tools: requiredSchema()}

	tools := client.chatTools([]domain.ToolID{"ticket"}, namesFor(engine.PlanInput{
		Tools: []domain.ToolID{"ticket"},
	}))
	if len(tools) < 1 {
		t.Fatal("no tools were built")
	}
	got := tools[0].Function.Parameters["required"]
	if !reflect.DeepEqual(got, []string{"subscriptionId"}) {
		t.Fatalf("required = %v, want [subscriptionId]", got)
	}
}

func requiredSchema() staticSchema {
	return staticSchema{id: "ticket", desc: "subscribe", schema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"subscriptionId": map[string]any{"type": "string"}},
		"required":   []any{"subscriptionId"},
	}}
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
