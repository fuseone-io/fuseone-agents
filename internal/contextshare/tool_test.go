package contextshare

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

func TestRead_returnsOnlyAContractedArtifact(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	content := engine.NewMemoryContent()
	body := []byte("root cause: queue saturation")
	ref, err := content.Put(ctx, "source-run", 4, body)
	if err != nil {
		t.Fatalf("store source: %v", err)
	}
	artifact := domain.ContextArtifact{
		Name: "triage_summary", Kind: "text", Ref: ref, Digest: digest(body),
		SourceRun: "source-run", SourceAgent: "triage",
		Labels: domain.NewLabels(domain.LabelUntrusted, domain.LabelArea(domain.Scope{
			Company: "acme", Area: "cx",
		})),
	}

	got, err := New(nil, nil, content).Invoke(ctx, engine.Call{
		RunID: "listener-run", Seq: 6, Tool: domain.ToolContextRead,
		Args:             []byte(`{"name":"triage_summary"}`),
		ContextArtifacts: []domain.ContextArtifact{artifact},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got.Failed {
		t.Fatalf("result failed with %s", got.ErrorCode)
	}
	readBack, err := content.Get(ctx, got.ResultRef)
	if err != nil {
		t.Fatalf("read copied result: %v", err)
	}
	if string(readBack) != string(body) {
		t.Fatalf("body = %q, want %q", readBack, body)
	}
	if !got.Labels.Has(domain.LabelUntrusted) || !got.Labels.Has(domain.LabelArea(domain.Scope{Company: "acme", Area: "cx"})) {
		t.Fatalf("labels = %v, want artifact labels", got.Labels)
	}
	if got.Context == nil || got.Context.SourceRun != "source-run" || got.Context.Digest != digest(body) {
		t.Fatalf("context provenance = %+v", got.Context)
	}
}

func TestRead_refusesArbitraryContentReferences(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	content := engine.NewMemoryContent()
	ref, err := content.Put(ctx, "source-run", 4, []byte("secret context"))
	if err != nil {
		t.Fatalf("store source: %v", err)
	}

	got, err := New(nil, nil, content).Invoke(ctx, engine.Call{
		RunID: "listener-run", Seq: 6, Tool: domain.ToolContextRead,
		Args: []byte(`{"name":"not-in-contract","ref":"` + ref + `"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !got.Failed || got.ErrorCode != CodeContextArtifactNotAllowed || got.ResultRef != "" {
		t.Fatalf("result = %+v, want a contract refusal without a content ref", got)
	}
}

func TestRead_refusesDigestMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	content := engine.NewMemoryContent()
	ref, err := content.Put(ctx, "source-run", 4, []byte("tampered"))
	if err != nil {
		t.Fatalf("store source: %v", err)
	}

	got, err := New(nil, nil, content).Invoke(ctx, engine.Call{
		RunID: "listener-run", Seq: 6, Tool: domain.ToolContextRead,
		Args: []byte(`{"name":"triage_summary"}`),
		ContextArtifacts: []domain.ContextArtifact{{
			Name: "triage_summary", Ref: ref, Digest: "sha256:expected",
			SourceRun: "source-run",
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !got.Failed || got.ErrorCode != CodeContextArtifactDigestMismatch || got.ResultRef != "" {
		t.Fatalf("result = %+v, want digest mismatch without a content ref", got)
	}
}
