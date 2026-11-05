package connectortools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/engine"
)

// EvidenceFetcher is the one remote surface: one GET against one source.
// Declared here, by the consumer.
type EvidenceFetcher interface {
	Fetch(ctx context.Context, cfg EvidenceConfig, credential SecretValue,
		request EvidenceRequest) ([]byte, error)
}

// WithEvidence enables evidence collection's native path.
func (l *Layer) WithEvidence(fetcher EvidenceFetcher) *Layer {
	l.evidence = fetcher
	return l
}

func (l *Layer) invokeEvidenceNative(
	ctx context.Context, instance Instance, op connectors.Operation, call engine.Call,
) (engine.ToolResult, error) {
	if l.evidence == nil {
		return failed(CodeConnectorUnavailable), nil
	}
	switch op.ID {
	case "evidence.check":
		return l.evidenceCheck(ctx, instance, call)
	case "evidence.collect":
		return l.evidenceCollect(ctx, instance, call)
	default:
		return failed(CodeConnectorUnavailable), nil
	}
}

// evidenceCheck runs the control over the shortest window the source accepts.
func (l *Layer) evidenceCheck(
	ctx context.Context, instance Instance, call engine.Call,
) (engine.ToolResult, error) {
	control, err := l.runEvidenceControl(ctx, instance, evidenceCheckSpan(instance.Evidence.Window, call))
	if err != nil {
		return evidenceFailure(err)
	}
	return l.storeJSON(ctx, call, evidenceLabels(), map[string]any{
		"operation": "evidence.check", "instance": instance.Name,
		"credential": "accepted", "control": control,
	})
}

func evidenceCheckSpan(window EvidenceWindow, call engine.Call) evidenceSpan {
	if window.Kind == EvidenceWindowPeriod {
		return evidencePeriodSpan(window.Periods[0], call.At)
	}
	// A day is arbitrary and enough: the question is whether anything answers.
	span := evidencePeriodSpan(86400, call.At)
	span.Kind, span.PeriodSeconds = EvidenceWindowRange, 0
	return span
}

func (l *Layer) evidenceCollect(
	ctx context.Context, instance Instance, call engine.Call,
) (engine.ToolResult, error) {
	args, ok := decodeEvidenceCollect(call.Args)
	if !ok {
		return failed(CodeConnectorBadArguments), nil
	}
	collection, code, err := l.prepareEvidence(ctx, instance, call, args)
	if err != nil {
		return evidenceFailure(err)
	}
	if code != "" {
		return failed(code), nil
	}
	// A finished collection has nothing left to page; asking again answers it.
	if collection.state.Coverage == "" || collection.state.Coverage == EvidencePartial {
		if err := collection.run(ctx); err != nil {
			return evidenceFailure(err)
		}
	}
	return l.finishEvidence(ctx, collection)
}

func (l *Layer) finishEvidence(
	ctx context.Context, collection *evidenceCollection,
) (engine.ToolResult, error) {
	if err := collection.blocks.flush(ctx); err != nil {
		return engine.ToolResult{}, err
	}
	state := collection.state
	state.Blocks = append(state.Blocks, collection.blocks.refs...)
	ref, err := l.sealEvidence(ctx, collection.call, collection.instance, state)
	if err != nil {
		return engine.ToolResult{}, err
	}
	return l.storeJSON(ctx, collection.call, evidenceLabels(), evidenceAnswer(collection.call, state, ref))
}

// prepareEvidence builds a call's work: a fresh collection or a resumed one.
// The error is the source's, the code is a refusal of the call itself.
func (l *Layer) prepareEvidence(
	ctx context.Context, instance Instance, call engine.Call, args evidenceCollectArgs,
) (*evidenceCollection, string, error) {
	if args.Resume != "" {
		state, code := l.openEvidence(ctx, call, instance, args.Resume)
		if code != "" {
			return nil, code, nil
		}
		return l.evidenceWork(instance, call, state, state.Params)
	}
	query, found := evidenceQueryByID(instance.Evidence, args.Query)
	if !found {
		return nil, CodeConnectorBadArguments, nil
	}
	span, ok := resolveEvidenceSpan(instance.Evidence.Window, query, args.Window, call.At)
	if !ok {
		return nil, CodeConnectorBadArguments, nil
	}
	state := evidenceState{
		Instance: instance.Name, Query: query.ID, Params: args.Params, Window: span,
		Contract: evidenceContractDigest(instance.Evidence, query),
	}
	if _, ok := evidenceFilters(query, args.Params); !ok {
		return nil, CodeConnectorBadArguments, nil
	}
	control, err := l.runEvidenceControl(ctx, instance, evidenceControlSpan(instance.Evidence.Window, span, call))
	if err != nil {
		return nil, "", err
	}
	state.Control = control
	return l.evidenceWork(instance, call, state, args.Params)
}

// evidenceControlSpan is the collection's window, or for a windowless lookup the
// shortest one, because the control always honours a window.
func evidenceControlSpan(window EvidenceWindow, span evidenceSpan, call engine.Call) evidenceSpan {
	if span.Kind == EvidenceWindowNone {
		return evidenceCheckSpan(window, call)
	}
	return span
}

func (l *Layer) evidenceWork(
	instance Instance, call engine.Call, state evidenceState, params map[string]string,
) (*evidenceCollection, string, error) {
	query, found := evidenceQueryByID(instance.Evidence, state.Query)
	if !found || state.Contract != evidenceContractDigest(instance.Evidence, query) {
		return nil, CodeConnectorContractChanged, nil
	}
	filters, ok := evidenceFilters(query, params)
	if !ok {
		return nil, CodeConnectorBadArguments, nil
	}
	return &evidenceCollection{
		l: l, call: call, instance: instance, query: query, filters: filters, state: state,
		blocks: evidenceBlockWriter{l: l, call: call}, seen: map[string]struct{}{},
	}, "", nil
}

func evidenceFailure(err error) (engine.ToolResult, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return engine.ToolResult{}, err
	}
	if errors.Is(err, errEvidenceRefused) {
		return failed(CodeConnectorQueryRefused), nil
	}
	var remote evidenceRemoteError
	if errors.As(err, &remote) {
		switch remote.status {
		case 401, 403:
			return failed(CodeConnectorUpstreamAuth), nil
		case 404:
			return failed(CodeConnectorUpstreamNotFound), nil
		}
	}
	return failed(CodeConnectorUpstreamFailed), nil
}

// evidenceContractDigest binds a decision to what would actually be read: the
// source, this query's declaration and the control that runs before it, whose
// answer decides whether absence can be concluded. Not the other queries beside it.
func evidenceContractDigest(cfg EvidenceConfig, query EvidenceQuery) string {
	bound := cfg
	bound.Queries = []EvidenceQuery{query}
	if control, found := evidenceQueryByID(cfg, cfg.Control); found && control.ID != query.ID {
		bound.Queries = append(bound.Queries, control)
	}
	raw, err := json.Marshal(bound)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// evidenceApprovalBinding is the digest a collect's approval travels with.
func evidenceApprovalBinding(instance Instance, op connectors.Operation, call engine.Call) string {
	if op.ID != "evidence.collect" {
		return ""
	}
	args, ok := decodeEvidenceCollect(call.Args)
	if !ok || args.Resume != "" {
		return ""
	}
	query, found := evidenceQueryByID(instance.Evidence, args.Query)
	if !found {
		return ""
	}
	return evidenceContractDigest(instance.Evidence, query)
}

// evidenceDescription tells a model what this instance can answer: the schema
// is per operation, and the queries are per instance.
func evidenceDescription(instance Instance, op connectors.Operation) string {
	if op.ID != "evidence.collect" {
		return op.Summary
	}
	var text strings.Builder
	text.WriteString(op.Summary)
	text.WriteString(" Queries:")
	for _, query := range instance.Evidence.Queries {
		fmt.Fprintf(&text, " %s", query.ID)
		if len(query.Parameters) > 0 {
			names := make([]string, 0, len(query.Parameters))
			for _, parameter := range query.Parameters {
				names = append(names, parameter.Name)
			}
			fmt.Fprintf(&text, "(%s)", strings.Join(names, ", "))
		}
		fmt.Fprintf(&text, ": %s", query.Description)
	}
	return text.String()
}

// Native schemas return the provider's property map; the wire adds the envelope.
func evidenceCheckSchema() map[string]any {
	return map[string]any{}
}

func evidenceCollectSchema() map[string]any {
	return map[string]any{
		"query": map[string]any{"type": "string",
			"description": "A query this instance declares. Omit when resuming."},
		"params": map[string]any{"type": "object",
			"additionalProperties": map[string]any{"type": "string"},
			"description":          "A value for every parameter of the query."},
		"window": map[string]any{"type": "object",
			"properties": map[string]any{
				"periodSeconds": map[string]any{"type": "integer", "minimum": 1,
					"description": "How far back, for a source with fixed periods; the next accepted one is used."},
				"from": map[string]any{"type": "string", "description": "RFC 3339 start, for a source with absolute ranges."},
				"to":   map[string]any{"type": "string", "description": "RFC 3339 end, for a source with absolute ranges."},
			},
			"additionalProperties": false,
			"description":          "The window. Omit for a lookup with no window, and when resuming."},
		"resume": map[string]any{"type": "string",
			"description": "The evidence reference of a partial collection, to continue it. Nothing else goes with it."},
	}
}
