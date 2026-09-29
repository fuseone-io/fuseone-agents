package model

import (
	"github.com/anthropics/anthropic-sdk-go"

	"github.com/fuseone/agents/internal/domain"
)

func (a *Anthropic) toolParams(ids []domain.ToolID, offered names) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(ids))
	for _, id := range ids {
		if isContextReadTool(id) {
			out = append(out, contextReadToolParam(offered))
			continue
		}
		if a.tools == nil {
			continue
		}
		_, desc, schema, ok := a.tools.Schema(id)
		if !ok {
			continue
		}
		tool := anthropic.ToolParam{
			Name:        offered.wire[id],
			Description: anthropic.String(desc),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: propertiesOf(schema)},
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return append(out, finishToolParam(offered))
}

func contextReadToolParam(offered names) anthropic.ToolUnionParam {
	tool := anthropic.ToolParam{
		Name:        offered.wire[domain.ToolContextRead],
		Description: anthropic.String(contextReadToolDescription),
		InputSchema: anthropic.ToolInputSchemaParam{Properties: contextReadToolSchema()},
	}
	return anthropic.ToolUnionParam{OfTool: &tool}
}

func finishToolParam(offered names) anthropic.ToolUnionParam {
	tool := anthropic.ToolParam{
		Name:        offered.wire[finishToolID],
		Description: anthropic.String(finishToolDescription),
		InputSchema: anthropic.ToolInputSchemaParam{Properties: finishToolSchema()},
	}
	return anthropic.ToolUnionParam{OfTool: &tool}
}

/*
propertiesOf reads a tool's fields, however its schema was written.

This provider is given the fields and builds the object around them. A schema
written as the whole object — which is what the other provider's wire format
takes — would arrive as a property called "type" and one called "properties",
and the request is refused for a schema that is invalid with no word about
which tool wrote it. Both shapes exist in this repository, so both are read
here rather than at each author.
*/
func propertiesOf(schema map[string]any) map[string]any {
	if schema["type"] != "object" {
		return schema
	}
	fields, ok := schema["properties"].(map[string]any)
	if !ok {
		return schema
	}
	return fields
}
