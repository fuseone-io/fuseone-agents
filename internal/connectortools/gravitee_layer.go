package connectortools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

func (l *Layer) invokeGraviteeNative(
	ctx context.Context, instance Instance, op connectors.Operation, call engine.Call,
) (engine.ToolResult, error) {
	args, ok := decodeGraviteeSubscriptionArgs(call.Args)
	if !ok {
		return failed(CodeConnectorBadArguments), nil
	}
	var result engine.ToolResult
	var err error
	switch op.ID {
	case "gravitee.inspect_subscription":
		if l.inspect == nil {
			return failed(CodeConnectorUnavailable), nil
		}
		result, err = l.inspect.Inspect(ctx, instance.Name, call, args)
	case "gravitee.accept_subscription":
		if l.accept == nil {
			return failed(CodeConnectorUnavailable), nil
		}
		result, err = l.accept.Accept(ctx, instance.Name, call, args)
	default:
		return failed(CodeConnectorUnavailable), nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return engine.ToolResult{}, err
	}
	if err != nil {
		return failed(graviteeErrorCode(err)), nil
	}
	return result, nil
}

func graviteeErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrGraviteeTTL):
		return CodeConnectorBadArguments
	case errors.Is(err, ErrGraviteeTicket), errors.Is(err, ErrGraviteeEvidence),
		errors.Is(err, ErrGraviteeRequester), errors.Is(err, ErrGraviteeState),
		errors.Is(err, ErrGraviteeScope), errors.Is(err, ticket.ErrAttemptConflict):
		return CodeConnectorSnapshotChanged
	case errors.Is(err, ErrNoGraviteeAuthority), errors.Is(err, ErrUnavailable):
		return CodeConnectorUnavailable
	default:
		return CodeConnectorUpstreamFailed
	}
}

func decodeGraviteeSubscriptionArgs(raw []byte) (GraviteeInspectInput, bool) {
	var args GraviteeInspectInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return GraviteeInspectInput{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return GraviteeInspectInput{}, false
	}
	if !graviteeID.MatchString(args.SubscriptionID) {
		return GraviteeInspectInput{}, false
	}
	if args.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, args.ExpiresAt)
		if err != nil {
			return GraviteeInspectInput{}, false
		}
		args.ExpiresAt = parsed.UTC().Format(time.RFC3339Nano)
	}
	return args, true
}

func graviteeSubscriptionSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"subscriptionId": map[string]any{
				"type": "string", "minLength": 1, "maxLength": 128,
				"description": "The Gravitee API subscription identifier from this ticket.",
			},
			"expiresAt": map[string]any{
				"type": "string", "format": "date-time",
				"description": "The requested API key expiration as a canonical RFC 3339 UTC instant.",
			},
		},
		"required": []string{"subscriptionId"},
	}
}
