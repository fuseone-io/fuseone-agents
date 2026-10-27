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
		if a.tools == nil {
			// A rig with no schema view at all; production always has one.
			continue
		}
		_, desc, schema, ok := a.tools.Schema(id)
		if err := staleDeclaration(id, declared[id], ok); err != nil {
			return nil, err
		}
		if !ok {
			// Declared by the engine and still absent: planned around.
			continue
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

/*
staleDeclaration is the catalogue moving between the engine's preflight and
request assembly, in either direction. A tool the engine saw that assembly
cannot describe would be silently omitted; a tool the engine declared absent
that assembly can describe again would be silently withheld — and the model
could finish the run without a tool it has back. Both abort before any HTTP,
and the retry's preflight classifies the new state (#150 review).
*/
func staleDeclaration(id domain.ToolID, declaredAbsent, describable bool) error {
	if !declaredAbsent && !describable {
		return fmt.Errorf(
			"model: tool %s vanished between the engine's preflight and request assembly", id)
	}
	if declaredAbsent && describable {
		return fmt.Errorf(
			"model: tool %s reappeared between the engine's preflight and request assembly", id)
	}
	return nil
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
