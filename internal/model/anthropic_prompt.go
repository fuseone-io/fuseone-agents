package model

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/fuseone/agents/internal/engine"
)

// system ends with a breakpoint covering tools and stable platform text.
func (a *Anthropic) system() []anthropic.TextBlockParam {
	blocks := []anthropic.TextBlockParam{{Text: a.cfg.SystemPrompt}, {Text: loopContract}}
	blocks[len(blocks)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()
	return blocks
}

// messages caches the ledger prefix and places changing guidance after it.
func (a *Anthropic) messages(in engine.PlanInput, offered names) []anthropic.MessageParam {
	messages := messagesFrom(in.Transcript, offered)
	cacheMessageTail(messages)
	if note := volatilePlanningNote(in, a.tools, offered); note != "" {
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(note)))
	}
	return messages
}

func volatilePlanningNote(in engine.PlanInput, schemas ToolSchemas, offered names) string {
	var notes []string
	if in.Step != "" {
		notes = append(notes, stepNote(in))
	}
	if note := memoryToolsNote(in, schemas, offered); note != "" {
		notes = append(notes, note)
	}
	if note := budgetNote(in); note != "" {
		notes = append(notes, note)
	}
	return strings.Join(notes, "\n\n")
}

func cacheMessageTail(messages []anthropic.MessageParam) {
	if len(messages) == 0 || len(messages[len(messages)-1].Content) == 0 {
		return
	}
	content := messages[len(messages)-1].Content
	cache := anthropic.NewCacheControlEphemeralParam()
	block := &content[len(content)-1]
	switch {
	case block.OfText != nil:
		block.OfText.CacheControl = cache
	case block.OfToolUse != nil:
		block.OfToolUse.CacheControl = cache
	case block.OfToolResult != nil:
		block.OfToolResult.CacheControl = cache
	}
}

// messagesFrom rebuilds provider messages from the ledger-derived transcript.
func messagesFrom(turns []engine.Turn, offered names) []anthropic.MessageParam {
	var out []anthropic.MessageParam
	for _, turn := range turns {
		out = appendAnthropicTurn(out, turn, offered)
	}
	if len(out) == 0 {
		out = append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(nothingSaid)))
	}
	return out
}

func appendAnthropicTurn(
	out []anthropic.MessageParam, turn engine.Turn, offered names,
) []anthropic.MessageParam {
	switch turn.Kind {
	case engine.TurnInput, engine.TurnNote:
		return append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(turn.Text)))
	case engine.TurnToolUse:
		var input any // Tool arguments are arbitrary JSON values on the provider wire.
		if len(turn.Args) > 0 {
			_ = json.Unmarshal(turn.Args, &input)
		}
		return append(out, anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock(turn.CallID, input, offered.wire[turn.Tool])))
	case engine.TurnToolResult:
		return append(out, anthropic.NewUserMessage(
			anthropic.NewToolResultBlock(turn.CallID, toolResultContent(turn), turn.Failed)))
	}
	return out
}
