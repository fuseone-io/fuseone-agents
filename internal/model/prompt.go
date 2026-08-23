package model

import (
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

const promptBreakdownUnit = "content_bytes"

/*
promptInputBreakdown measures the content the platform put in front of the
model, grouped by where it came from.

It deliberately does not say "tokens". Tokenisation belongs to the provider
and the model generation; without that tokeniser, splitting the provider's
total afterwards would be an estimate presented as fact. This measures exact
content bytes at the boundary we own, which is enough to tell whether a run is
large because of instructions, channel input, platform notes, arguments or
tool results.
*/
func promptInputBreakdown(
	in engine.PlanInput, cfg Config, toolSchemas ToolSchemas, offered names,
) domain.PromptInputBreakdown {
	out := domain.PromptInputBreakdown{
		Unit:                promptBreakdownUnit,
		Instructions:        int64(len(cfg.SystemPrompt)),
		Platform:            int64(len(loopContract)),
		ToolArgumentsByTool: map[domain.ToolID]int64{},
		ToolResultsByTool:   map[domain.ToolID]int64{},
	}

	if in.Step != "" {
		out.Platform += int64(len(stepNote(in)))
	}
	if note := budgetNote(in); note != "" {
		out.Platform += int64(len(note))
	}
	out.ToolSchemas = toolSchemaBytes(in.Tools, toolSchemas, offered)

	for _, turn := range in.Transcript {
		switch turn.Kind {
		case engine.TurnInput:
			out.Input += int64(len(turn.Text))
		case engine.TurnNote:
			out.Notes += int64(len(turn.Text))
		case engine.TurnToolUse:
			n := int64(len(turn.Args))
			out.ToolArguments += n
			out.ToolArgumentsByTool[turn.Tool] += n
		case engine.TurnToolResult:
			n := int64(len(toolResultContent(turn)))
			out.ToolResults += n
			out.ToolResultsByTool[turn.Tool] += n
		}
	}
	if len(in.Transcript) == 0 {
		out.Platform += int64(len(nothingSaid))
	}

	out.Total = out.Instructions + out.Platform + out.Input + out.Notes +
		out.ToolSchemas + out.ToolArguments + out.ToolResults
	if len(out.ToolArgumentsByTool) == 0 {
		out.ToolArgumentsByTool = nil
	}
	if len(out.ToolResultsByTool) == 0 {
		out.ToolResultsByTool = nil
	}
	return out
}

func toolSchemaBytes(ids []domain.ToolID, schemas ToolSchemas, offered names) int64 {
	if schemas == nil {
		return 0
	}
	var total int64
	for _, id := range ids {
		_, desc, schema, ok := schemas.Schema(id)
		if !ok {
			continue
		}
		total += int64(len(offered.wire[id]) + len(desc))
		if len(schema) > 0 {
			raw, err := json.Marshal(schema)
			if err == nil {
				total += int64(len(raw))
			}
		}
	}
	return total
}

func toolResultContent(t engine.Turn) string {
	if len(t.Content) == 0 {
		return "(no content)"
	}
	return string(t.Content)
}

func budgetNote(in engine.PlanInput) string {
	if in.Budget.Micros <= 0 {
		return ""
	}
	return "Budget remaining for this run: " + formatMicros(in.Remaining.Micros) +
		". Pace yourself and finish cleanly rather than being cut off."
}
