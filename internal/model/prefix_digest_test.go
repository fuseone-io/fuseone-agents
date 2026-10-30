package model_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

// An agent that never read its cache could not be diagnosed: the ledger kept
// each prompt block's size, and two blocks of equal size with different bytes
// read the same. A fingerprint of the tools and stable system blocks, as sent,
// names the block that moved between two calls.
func TestPlan_prefixDigests_nameTheBlockThatChanged(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		kind     model.Kind
		response string
	}{
		{"anthropic", model.KindAnthropic, anthropicToolUse},
		{"openai compatible", model.KindOpenAICompatible, openAIToolCall},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			digestsAt := func(step string) domain.PromptDigests {
				c := serve(t, tc.response)
				in := input()
				in.Step = step
				p, err := plannerFor(t, tc.kind, c.server.URL, model.Config{
					SystemPrompt: "Você varre os logs de integração.",
				}).Plan(context.Background(), in)
				if err != nil {
					t.Fatalf("Plan at %q: %v", step, err)
				}
				if p.Prompt.Digests == nil {
					t.Fatal("the planned prompt carries no digests")
				}
				return *p.Prompt.Digests
			}

			first, again, moved := digestsAt("Run V1"), digestsAt("Run V1"), digestsAt("Run V2")
			if first.Tools == "" || len(first.System) == 0 {
				t.Fatalf("digests = %+v, want the tools and every system block", first)
			}
			if !equalDigests(first, again) {
				t.Errorf("the same request digested differently: %+v vs %+v", first, again)
			}
			if first.Tools != moved.Tools {
				t.Errorf("a step change moved the tools digest: %s vs %s", first.Tools, moved.Tools)
			}
			if equalDigests(first, moved) {
				t.Errorf("a step change left every digest equal: %+v", first)
			}
		})
	}
}

func equalDigests(a, b domain.PromptDigests) bool {
	if a.Tools != b.Tools || len(a.System) != len(b.System) {
		return false
	}
	for i := range a.System {
		if a.System[i] != b.System[i] {
			return false
		}
	}
	return true
}

// The digests promise the bytes the provider's cache sees, not a stable
// rendering of the same values. Anything else deterministic — a re-encoding,
// a partial rebuild, the wrong object — would pass every equality check above
// and still fingerprint something that never went on the wire. So the request
// is read back raw: tools and each system block as json.RawMessage, never
// decoded and re-encoded, and hashed independently.
func TestPlan_prefixDigests_matchTheRawRequestBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		kind     model.Kind
		response string
		system   func(t *testing.T, raw []byte) []json.RawMessage
	}{
		{"anthropic", model.KindAnthropic, anthropicToolUse, anthropicSystemRaw},
		{"openai compatible", model.KindOpenAICompatible, openAIToolCall, openAISystemRaw},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var sent atomic.Pointer[[]byte]
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				sent.Store(&raw)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.response)
			}))
			t.Cleanup(srv.Close)

			in := input()
			in.Step = "Run V1"
			p, err := plannerFor(t, tc.kind, srv.URL, model.Config{SystemPrompt: "Você varre os logs."}).
				Plan(context.Background(), in)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			raw := sent.Load()
			if raw == nil || p.Prompt.Digests == nil {
				t.Fatalf("request %v, digests %v: want both", raw != nil, p.Prompt.Digests)
			}

			var wire struct {
				Tools json.RawMessage `json:"tools"`
			}
			if err := json.Unmarshal(*raw, &wire); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			want := domain.PromptDigests{Tools: rawDigest(wire.Tools)}
			for _, block := range tc.system(t, *raw) {
				want.System = append(want.System, rawDigest(block))
			}
			if !equalDigests(*p.Prompt.Digests, want) {
				t.Errorf("recorded %+v, the wire hashes to %+v", *p.Prompt.Digests, want)
			}
		})
	}
}

func anthropicSystemRaw(t *testing.T, raw []byte) []json.RawMessage {
	t.Helper()
	var wire struct {
		System []json.RawMessage `json:"system"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decode system: %v", err)
	}
	return wire.System
}

func openAISystemRaw(t *testing.T, raw []byte) []json.RawMessage {
	t.Helper()
	var wire struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	var out []json.RawMessage
	for _, msg := range wire.Messages {
		var role struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(msg, &role); err != nil || role.Role != "system" {
			break
		}
		out = append(out, msg)
	}
	return out
}

func rawDigest(raw json.RawMessage) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:16]
}
