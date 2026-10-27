package model

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

func (a *Anthropic) toolParams(in engine.PlanInput, offered names) ([]anthropic.ToolUnionParam, error) {
	declared := declaredUnavailable(in)
	out := make([]anthropic.ToolUnionParam, 0, len(in.Tools))
	for _, id := range in.Tools {
		if isContextReadTool(id) {
			out = append(out, contextReadToolParam(offered))
			continue
		}
		if declared[id] {
			// The engine resolved this absence and told the model about it.
			continue
		}
		if a.tools == nil {
			// A rig with no schema view at all; production always has one.
			continue
		}
		_, desc, schema, ok := a.tools.Schema(id)
		if !ok {
			return nil, vanishedTool(id)
		}
		fields, required := propertiesOf(schema)
		tool := anthropic.ToolParam{
			Name:        offered.wire[id],
			Description: anthropic.String(desc),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: fields, Required: required},
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return append(out, finishToolParam(offered)), nil
}

// vanishedTool is a schema present at the engine's preflight and gone at
// request assembly — the catalogue moved between two reads. Failing before
// any HTTP is what keeps the omission loud: the retry's preflight sees the
// new state and classifies it properly (#150 review).
func vanishedTool(id domain.ToolID) error {
	return fmt.Errorf(
		"model: tool %s vanished between the engine's preflight and request assembly", id)
}

func declaredUnavailable(in engine.PlanInput) map[domain.ToolID]bool {
	if len(in.Unavailable) == 0 {
		return nil
	}
	out := make(map[domain.ToolID]bool, len(in.Unavailable))
	for _, id := range in.Unavailable {
		out[id] = true
	}
	return out
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
