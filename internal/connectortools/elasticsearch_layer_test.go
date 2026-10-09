package connectortools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

type fakeESSearch struct {
	bodies   []string
	indices  []string
	response string
	err      error
}

func (f *fakeESSearch) Search(
	_ context.Context, _ ElasticsearchConfig, _ SecretValue, index string, body []byte,
) ([]byte, error) {
	f.indices = append(f.indices, index)
	f.bodies = append(f.bodies, string(body))
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.response), nil
}

func esLayer(t *testing.T, remote *fakeESSearch) *Layer {
	t.Helper()
	layer := New(nil, nil, engine.NewMemoryContent(), nil).
		WithElasticsearch(remote)
	instance := validElasticsearchInstance()
	instance.Token = "CANARY-password"
	layer.SetInstances([]Instance{instance})
	return layer
}

const esAggResponse = `{
  "took": 391, "hits": {"total": 1615},
  "aggregations": {"por_ip": {"buckets": [
    {"key": "198.51.100.7", "doc_count": 86,
     "por_dia": {"buckets": [{"key_as_string": "2026-10-09", "doc_count": 86}]},
     "por_status": {"buckets": [{"key": 200, "doc_count": 80}, {"key": 422, "doc_count": 6}]}}
  ]}}}`

func topIPsCall(args string) engine.Call {
	return engine.Call{Tool: "elasticsearch.gw-logs.top_ips", RunID: "run-1", Seq: 4, Args: []byte(args)}
}

// The named query runs against the configured index with a platform-built
// body: the 6.x dialect, the mapped fields, the window from the arguments —
// and the result is the projection, labeled untrusted, because log content
// is data the taint check must keep seeing.
func TestTopIPs_buildsTheNamedQueryAndProjects(t *testing.T) {
	t.Parallel()
	remote := &fakeESSearch{response: esAggResponse}
	layer := esLayer(t, remote)
	result, err := layer.Invoke(t.Context(),
		topIPsCall(`{"path":"/customers/","match":"prefix","windowHours":168}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result = %+v", result)
	}
	if len(remote.indices) != 1 || remote.indices[0] != "logs-*" {
		t.Fatalf("indices = %v, want the configured pattern", remote.indices)
	}
	body := remote.bodies[0]
	for _, must := range []string{`"interval":"1d"`, `"prefix"`, `"uri"`, `"remote-address"`, `"now-168h"`} {
		if !strings.Contains(body, must) {
			t.Fatalf("body misses %s: %s", must, body)
		}
	}
	if strings.Contains(body, "CANARY") {
		t.Fatal("the password reached the query body")
	}
	if !result.Labels.Has(domain.LabelUntrusted) {
		t.Fatal("log content came back without the untrusted label")
	}
	raw, _ := layer.content.Get(t.Context(), result.ResultRef)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ips, _ := out["ips"].([]any)
	if out["total"] != float64(1615) || len(ips) != 1 {
		t.Fatalf("projection = %+v", out)
	}
}

// Arguments are values, never structure: an unknown field — index included —
// is refused before anything reaches the wire.
func TestTopIPs_argumentsNeverChooseStructure(t *testing.T) {
	t.Parallel()
	remote := &fakeESSearch{response: esAggResponse}
	layer := esLayer(t, remote)
	for name, args := range map[string]string{
		"index smuggled": `{"path":"/x","match":"exact","windowHours":1,"index":"secrets-*"}`,
		"dsl smuggled":   `{"path":"/x","match":"exact","windowHours":1,"query":{}}`,
		"bad match":      `{"path":"/x","match":"regexp","windowHours":1}`,
		"no path":        `{"match":"exact","windowHours":1}`,
		"control chars":  "{\"path\":\"/x\\u0000y\",\"match\":\"exact\",\"windowHours\":1}",
	} {
		t.Run(name, func(t *testing.T) {
			result, err := layer.Invoke(t.Context(), topIPsCall(args))
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if !result.Failed || result.ErrorCode != CodeConnectorBadArguments {
				t.Fatalf("result = %+v, want bad arguments", result)
			}
		})
	}
	if len(remote.bodies) != 0 {
		t.Fatal("a refused call reached the wire")
	}
}

// The window never exceeds the instance's ceiling.
func TestTopIPs_pastTheWindowCeiling_isRefused(t *testing.T) {
	t.Parallel()
	remote := &fakeESSearch{response: esAggResponse}
	layer := esLayer(t, remote)
	result, err := layer.Invoke(t.Context(),
		topIPsCall(`{"path":"/x","match":"exact","windowHours":169}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
		t.Fatalf("result = %+v, want guard refusal", result)
	}
}

const esHitsResponse = `{
  "took": 13, "hits": {"total": 2, "hits": [
    {"_source": {"@timestamp": "2026-10-09T18:05:38Z", "remote-address": "198.51.100.7",
      "status": 200, "method": 3, "response-time": 39,
      "api-key": "SECRET-SHAPED", "payload": "card=4111"}},
    {"_source": {"@timestamp": "2026-10-09T18:01:00Z", "remote-address": "198.51.100.8",
      "status": 404, "method": 3, "response-time": 2}}
  ]}}`

// The projection is an allowlist: fields outside it never reach the run,
// whatever the remote document carries.
func TestResourceHistory_projectsOnlyTheAllowedFields(t *testing.T) {
	t.Parallel()
	remote := &fakeESSearch{response: esHitsResponse}
	layer := esLayer(t, remote)
	result, err := layer.Invoke(t.Context(), engine.Call{
		Tool: "elasticsearch.gw-logs.resource_history", RunID: "run-1", Seq: 5,
		Args: []byte(`{"path":"/customers/42","windowHours":24}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result = %+v", result)
	}
	raw, _ := layer.content.Get(t.Context(), result.ResultRef)
	if strings.Contains(string(raw), "SECRET-SHAPED") || strings.Contains(string(raw), "card=") {
		t.Fatalf("a field outside the projection leaked: %s", raw)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	hits, _ := out["requests"].([]any)
	if len(hits) != 2 {
		t.Fatalf("projection = %+v", out)
	}
	first, _ := hits[0].(map[string]any)
	if first["ip"] != "198.51.100.7" || first["status"] != float64(200) {
		t.Fatalf("first = %+v", first)
	}
}

// Upstream refusals say their class, and errors never carry remote bodies.
func TestElasticsearch_upstreamFailures_areClassified(t *testing.T) {
	t.Parallel()
	for status, code := range map[int]string{
		401: CodeConnectorUpstreamAuth,
		404: CodeConnectorUpstreamNotFound,
		500: CodeConnectorUpstreamFailed,
	} {
		remote := &fakeESSearch{err: esRemoteError{status: status}}
		layer := esLayer(t, remote)
		result, err := layer.Invoke(t.Context(),
			topIPsCall(`{"path":"/x","match":"exact","windowHours":1}`))
		if err != nil {
			t.Fatalf("Invoke(%d): %v", status, err)
		}
		if !result.Failed || result.ErrorCode != code {
			t.Fatalf("status %d: result = %+v, want %s", status, result, code)
		}
	}
}
