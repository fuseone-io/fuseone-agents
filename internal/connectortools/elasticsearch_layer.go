package connectortools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// ElasticsearchSearcher is the one remote surface: run one search body
// against one index. Declared here, by the consumer.
type ElasticsearchSearcher interface {
	Search(ctx context.Context, cfg ElasticsearchConfig, credential SecretValue,
		index string, body []byte) ([]byte, error)
}

// WithElasticsearch enables the governed window's native path.
func (l *Layer) WithElasticsearch(searcher ElasticsearchSearcher) *Layer {
	l.elasticsearch = searcher
	return l
}

const (
	maxESPathChars = 512
	maxESTopSize   = 50
	maxESHistory   = 50
)

type esTopIPsArgs struct {
	Path        string `json:"path"`
	Match       string `json:"match"`
	WindowHours int    `json:"windowHours"`
	Size        int    `json:"size,omitempty"`
}

type esHistoryArgs struct {
	Path        string `json:"path"`
	WindowHours int    `json:"windowHours"`
}

func (l *Layer) invokeElasticsearchNative(
	ctx context.Context, instance Instance, op connectors.Operation, call engine.Call,
) (engine.ToolResult, error) {
	if l.elasticsearch == nil {
		return failed(CodeConnectorUnavailable), nil
	}
	credential := SecretValue{value: instance.Token}
	switch op.ID {
	case "elasticsearch.top_ips":
		args, ok := decodeESTopIPs(call.Args)
		if !ok {
			return failed(CodeConnectorBadArguments), nil
		}
		return l.esTopIPs(ctx, instance, credential, call, args)
	case "elasticsearch.resource_history":
		args, ok := decodeESHistory(call.Args)
		if !ok {
			return failed(CodeConnectorBadArguments), nil
		}
		return l.esHistory(ctx, instance, credential, call, args)
	default:
		return failed(CodeConnectorUnavailable), nil
	}
}

/*
esTopIPs is the named aggregation: the busiest addresses on a path.

The body is built here, from configuration and validated values — a model
supplies a path, a match kind and a window, never structure. The dialect is
the 6.x one (`interval`), which 7.x still accepts.
*/
func (l *Layer) esTopIPs(
	ctx context.Context, instance Instance, credential SecretValue,
	call engine.Call, args esTopIPsArgs,
) (engine.ToolResult, error) {
	cfg := instance.Elasticsearch
	if args.WindowHours > elasticsearchWindowCap(cfg) {
		return failed(CodeConnectorGuardRefused), nil
	}
	size := args.Size
	if size == 0 {
		size = 20
	}
	pathField := elasticsearchField(cfg.PathField, defaultESPathField)
	ipField := elasticsearchField(cfg.IPField, defaultESIPField)
	tsField := elasticsearchField(cfg.TimestampField, defaultESTimestampField)
	statusField := elasticsearchField(cfg.StatusField, defaultESStatusField)

	body := map[string]any{
		"size": 0,
		"query": map[string]any{"bool": map[string]any{"filter": []any{
			map[string]any{"range": map[string]any{tsField: map[string]any{
				"gte": fmt.Sprintf("now-%dh", args.WindowHours)}}},
			map[string]any{args.Match: map[string]any{pathField: args.Path}},
		}}},
		"aggs": map[string]any{"por_ip": map[string]any{
			"terms": map[string]any{"field": ipField, "size": size,
				"order": map[string]any{"_count": "desc"}},
			"aggs": map[string]any{
				"por_dia": map[string]any{"date_histogram": map[string]any{
					"field": tsField, "interval": "1d", "format": "yyyy-MM-dd"}},
				"por_status": map[string]any{"terms": map[string]any{
					"field": statusField, "size": 8}},
			},
		}},
	}
	raw, err := l.esSearch(ctx, instance, credential, body)
	if err != nil {
		return esFailure(err)
	}
	projected, err := projectESTopIPs(raw)
	if err != nil {
		return failed(CodeConnectorUpstreamFailed), nil
	}
	return l.storeJSON(ctx, call, domain.NewLabels(domain.LabelUntrusted), projected)
}

func (l *Layer) esHistory(
	ctx context.Context, instance Instance, credential SecretValue,
	call engine.Call, args esHistoryArgs,
) (engine.ToolResult, error) {
	cfg := instance.Elasticsearch
	if args.WindowHours > elasticsearchWindowCap(cfg) {
		return failed(CodeConnectorGuardRefused), nil
	}
	pathField := elasticsearchField(cfg.PathField, defaultESPathField)
	ipField := elasticsearchField(cfg.IPField, defaultESIPField)
	tsField := elasticsearchField(cfg.TimestampField, defaultESTimestampField)
	statusField := elasticsearchField(cfg.StatusField, defaultESStatusField)

	body := map[string]any{
		"size": maxESHistory,
		"query": map[string]any{"bool": map[string]any{"filter": []any{
			map[string]any{"range": map[string]any{tsField: map[string]any{
				"gte": fmt.Sprintf("now-%dh", args.WindowHours)}}},
			map[string]any{"term": map[string]any{pathField: args.Path}},
		}}},
		"sort": []any{map[string]any{tsField: map[string]any{"order": "desc"}}},
		// The projection starts at the source filter and ends at the
		// allowlist below: a remote document's other fields — payloads,
		// keys, whatever the pipeline ingested — never reach a run.
		"_source": []string{tsField, ipField, statusField, "method", "response-time"},
	}
	raw, err := l.esSearch(ctx, instance, credential, body)
	if err != nil {
		return esFailure(err)
	}
	projected, err := projectESHistory(raw, tsField, ipField, statusField)
	if err != nil {
		return failed(CodeConnectorUpstreamFailed), nil
	}
	return l.storeJSON(ctx, call, domain.NewLabels(domain.LabelUntrusted), projected)
}

func (l *Layer) esSearch(
	ctx context.Context, instance Instance, credential SecretValue, body map[string]any,
) ([]byte, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("connector: encode search: %w", err)
	}
	return l.elasticsearch.Search(ctx, instance.Elasticsearch, credential,
		instance.Elasticsearch.Index, encoded)
}

// projectESTopIPs keeps the aggregation's shape and nothing else.
func projectESTopIPs(raw []byte) (map[string]any, error) {
	var decoded struct {
		Took float64 `json:"took"`
		Hits struct {
			Total json.RawMessage `json:"total"`
		} `json:"hits"`
		Aggregations struct {
			PorIP struct {
				Buckets []struct {
					Key      any     `json:"key"`
					DocCount float64 `json:"doc_count"`
					PorDia   struct {
						Buckets []struct {
							KeyAsString string  `json:"key_as_string"`
							DocCount    float64 `json:"doc_count"`
						} `json:"buckets"`
					} `json:"por_dia"`
					PorStatus struct {
						Buckets []struct {
							Key      any     `json:"key"`
							DocCount float64 `json:"doc_count"`
						} `json:"buckets"`
					} `json:"por_status"`
				} `json:"buckets"`
			} `json:"por_ip"`
		} `json:"aggregations"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	ips := make([]map[string]any, 0, len(decoded.Aggregations.PorIP.Buckets))
	for _, bucket := range decoded.Aggregations.PorIP.Buckets {
		byDay := map[string]float64{}
		for _, day := range bucket.PorDia.Buckets {
			byDay[day.KeyAsString] = day.DocCount
		}
		byStatus := map[string]float64{}
		for _, status := range bucket.PorStatus.Buckets {
			byStatus[fmt.Sprint(status.Key)] = status.DocCount
		}
		ips = append(ips, map[string]any{
			"ip": fmt.Sprint(bucket.Key), "count": bucket.DocCount,
			"byDay": byDay, "byStatus": byStatus,
		})
	}
	return map[string]any{
		"operation": "elasticsearch.top_ips",
		"total":     esTotal(decoded.Hits.Total), "tookMs": decoded.Took,
		"ips": ips,
	}, nil
}

func projectESHistory(raw []byte, tsField, ipField, statusField string) (map[string]any, error) {
	var decoded struct {
		Hits struct {
			Total json.RawMessage `json:"total"`
			Hits  []struct {
				Source map[string]any `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	requests := make([]map[string]any, 0, len(decoded.Hits.Hits))
	for _, hit := range decoded.Hits.Hits {
		// The allowlist, field by field. _source filtering upstream is an
		// optimisation; this is the guarantee.
		requests = append(requests, map[string]any{
			"at":             hit.Source[tsField],
			"ip":             hit.Source[ipField],
			"status":         hit.Source[statusField],
			"method":         hit.Source["method"],
			"responseTimeMs": hit.Source["response-time"],
		})
	}
	return map[string]any{
		"operation": "elasticsearch.resource_history",
		"total":     esTotal(decoded.Hits.Total), "requests": requests,
	}, nil
}

// esTotal reads the 6.x number or the 7.x {value, relation} object.
func esTotal(raw json.RawMessage) float64 {
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var obj struct {
		Value float64 `json:"value"`
	}
	_ = json.Unmarshal(raw, &obj)
	return obj.Value
}

func esFailure(err error) (engine.ToolResult, error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return engine.ToolResult{}, err
	}
	var remote esRemoteError
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

func decodeESTopIPs(raw []byte) (esTopIPsArgs, bool) {
	var args esTopIPsArgs
	if !decodeStrict(raw, &args) {
		return esTopIPsArgs{}, false
	}
	if !validESPath(args.Path) ||
		(args.Match != "prefix" && args.Match != "exact") ||
		args.WindowHours < 1 ||
		args.Size < 0 || args.Size > maxESTopSize {
		return esTopIPsArgs{}, false
	}
	return args, true
}

func decodeESHistory(raw []byte) (esHistoryArgs, bool) {
	var args esHistoryArgs
	if !decodeStrict(raw, &args) {
		return esHistoryArgs{}, false
	}
	if !validESPath(args.Path) || args.WindowHours < 1 {
		return esHistoryArgs{}, false
	}
	return args, true
}

func decodeStrict(raw []byte, into any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return false
	}
	return errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

func validESPath(path string) bool {
	if path == "" || len(path) > maxESPathChars {
		return false
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return !strings.ContainsAny(path, "\x00")
}

func elasticsearchTopIPsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string",
				"description": "The path to aggregate, matched against the instance's configured path field."},
			"match": map[string]any{"type": "string", "enum": []string{"prefix", "exact"},
				"description": "Whether path is a prefix of the stored value or the exact value."},
			"windowHours": map[string]any{"type": "integer", "minimum": 1,
				"description": "How far back to look, capped by the instance's ceiling."},
			"size": map[string]any{"type": "integer", "minimum": 1, "maximum": maxESTopSize,
				"description": "How many addresses to return; default 20."},
		},
		"required":             []string{"path", "match", "windowHours"},
		"additionalProperties": false,
	}
}

func elasticsearchHistorySchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string",
				"description": "The exact stored path whose recent requests to list."},
			"windowHours": map[string]any{"type": "integer", "minimum": 1,
				"description": "How far back to look, capped by the instance's ceiling."},
		},
		"required":             []string{"path", "windowHours"},
		"additionalProperties": false,
	}
}
