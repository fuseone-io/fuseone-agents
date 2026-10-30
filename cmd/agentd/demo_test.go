package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

// The in-memory demo has no database and so no identity. It used to hand the
// seeding a content store built from that missing identity and panic before
// serving anything (#77). The store it gets instead has to be the one the
// console reads, or every finished demo run shows an answer nobody can open.
func TestDemoContent_withoutADatabase_seedsAnswersTheSameStoreResolves(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := ledger.NewMemory()
	content := demoContent(nil)

	if err := seedDemo(ctx, store, content); err != nil {
		t.Fatalf("seed demo without a database: %v", err)
	}

	steps, err := store.Read(ctx, "run-triage-8801", 0)
	if err != nil {
		t.Fatalf("read the seeded run: %v", err)
	}
	for _, step := range steps {
		if step.Kind != domain.StepRunFinished {
			continue
		}
		var finished domain.RunFinishedPayload
		if err := json.Unmarshal(step.Payload, &finished); err != nil {
			t.Fatalf("decode the finished payload: %v", err)
		}
		if finished.OutcomeRef == "" {
			t.Fatal("the finished demo run carries no answer reference")
		}
		if _, err := content.Get(ctx, finished.OutcomeRef); err != nil {
			t.Fatalf("the answer the seed wrote does not resolve in the same store: %v", err)
		}
		return
	}
	t.Fatal("the seeded triage run never finished")
}
