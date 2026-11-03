package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The board tools take their arguments raw and decode them here, strictly.
// The SDK's inferred schema does set additionalProperties:false, and the SDK
// then rejects an extra property, but with a prose error and no structured
// result. Here an extra, missing or non-string property is invalid_input,
// like every other malformed argument, and encoding/json's case-insensitive
// key matching never turns "Item_ID" into item_id.
func registerMoveCard(server *mcp.Server, b *Board) {
	server.AddTool(&mcp.Tool{
		Name: ToolMoveCard,
		Description: "Move a card of the factory board to another lane by changing only its Status. " +
			"Allowed only along the calling role's row of the transition table; never out of Travado " +
			"or Done. Returns structured state only.",
		InputSchema: objectSchema(map[string]propertySchema{
			"item_id": {Type: "string", Pattern: itemPattern.String(), Description: "the card's project item node id (PVTI_...)"},
			"lane":    {Type: "string", Enum: names(Lanes()), Description: "the lane to move the card to"},
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := strictArgs(req.Params.Arguments, "item_id", "lane")
		if !ok {
			return toolResult(MoveResult{Reason: ReasonInvalidInput}, true)
		}
		res, err := b.Move(callContext(ctx, req), MoveInput{ItemID: args["item_id"], Lane: args["lane"]})
		if err != nil {
			return errorResult(err)
		}
		return toolResult(res, !res.Moved && res.Reason != ReasonAlreadyInLane)
	})
}

func registerSetCardFields(server *mcp.Server, b *Board) {
	server.AddTool(&mcp.Tool{
		Name: ToolSetCardFields,
		Description: "Set a factory card's Work type and Size, and nothing else. Refused on a card " +
			"in Travado or Done. A field already holding the value is not written. Returns structured state only.",
		InputSchema: objectSchema(map[string]propertySchema{
			"item_id":   {Type: "string", Pattern: itemPattern.String(), Description: "the card's project item node id (PVTI_...)"},
			"work_type": {Type: "string", Enum: names(WorkTypes()), Description: "the board's Work type"},
			"size":      {Type: "string", Enum: names(Sizes()), Description: "the board's Size"},
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := strictArgs(req.Params.Arguments, "item_id", "work_type", "size")
		if !ok {
			return toolResult(FieldsResult{Reason: ReasonInvalidInput}, true)
		}
		res, err := b.SetFields(callContext(ctx, req), FieldsInput{ItemID: args["item_id"], WorkType: args["work_type"], Size: args["size"]})
		if err != nil {
			return errorResult(err)
		}
		return toolResult(res, res.Reason != ReasonUpdated && res.Reason != ReasonAlreadySet)
	})
}

// strictArgs accepts a JSON object holding exactly keys, each a string.
func strictArgs(raw json.RawMessage, keys ...string) (map[string]string, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) != len(keys) {
		return nil, false
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		v, present := fields[k]
		var s string
		if !present || json.Unmarshal(v, &s) != nil {
			return nil, false
		}
		out[k] = s
	}
	return out, true
}

type propertySchema struct {
	Type        string   `json:"type"`
	Pattern     string   `json:"pattern,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Description string   `json:"description,omitempty"`
}

type inputSchema struct {
	Type                 string                    `json:"type"`
	Properties           map[string]propertySchema `json:"properties"`
	Required             []string                  `json:"required"`
	AdditionalProperties bool                      `json:"additionalProperties"`
}

// objectSchema is what tools/list shows: every property required, none
// other allowed. It informs the caller; strictArgs is what enforces it.
func objectSchema(props map[string]propertySchema) json.RawMessage {
	s := inputSchema{Type: "object", Properties: props, Required: slices.Sorted(maps.Keys(props))}
	raw, err := json.Marshal(s)
	if err != nil {
		// A struct of strings always marshals; this cannot happen.
		panic(fmt.Sprintf("factory guard: input schema: %v", err))
	}
	return raw
}

// toolResult carries the result as structured content and as its JSON text.
func toolResult[T any](res T, failed bool) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("factory guard: encode result: %w", err)
	}
	return &mcp.CallToolResult{
		IsError:           failed,
		StructuredContent: json.RawMessage(raw),
		Content:           []mcp.Content{&mcp.TextContent{Text: string(raw)}},
	}, nil
}

// errorResult reports a failed read as a failed call. The error carries the
// method, the path the guard built and a status or count, never a body.
func errorResult(err error) (*mcp.CallToolResult, error) {
	var res mcp.CallToolResult
	res.SetError(err)
	return &res, nil
}
