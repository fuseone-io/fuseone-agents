package model

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// toolUses is how many tool_use blocks a response holds, the finish included: a judge that
// finished and also asked for a tool has not answered (NT-016).
func toolUses(resp *anthropic.Message) int {
	n := 0
	for _, block := range resp.Content {
		if _, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			n++
		}
	}
	return n
}

// allText is every text block of a response, before and after any tool use: what the model
// wrote beside its call, which a judge's closer reads (NT-016).
func allText(resp *anthropic.Message) string {
	var text strings.Builder
	for _, block := range resp.Content {
		if variant, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(variant.Text)
		}
	}
	return text.String()
}
