package dedupe_test

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/dedupe"
	"github.com/fuseone/agents/internal/domain"
)

func TestKeyValidate_requiresPlatformOwnedPrefix(t *testing.T) {
	tests := []struct {
		name string
		key  dedupe.Key
	}{
		{name: "missing company", key: dedupe.Key{
			Scope: domain.Scope{Area: "ops"}, AgentID: "triage",
			Tool: "github.create_issue", Fingerprint: "sha256:abc",
		}},
		{name: "missing area", key: dedupe.Key{
			Scope: domain.Scope{Company: "acme"}, AgentID: "triage",
			Tool: "github.create_issue", Fingerprint: "sha256:abc",
		}},
		{name: "missing agent", key: dedupe.Key{
			Scope: domain.Scope{Company: "acme", Area: "ops"},
			Tool:  "github.create_issue", Fingerprint: "sha256:abc",
		}},
		{name: "missing tool", key: dedupe.Key{
			Scope: domain.Scope{Company: "acme", Area: "ops"}, AgentID: "triage",
			Fingerprint: "sha256:abc",
		}},
		{name: "missing fingerprint", key: dedupe.Key{
			Scope: domain.Scope{Company: "acme", Area: "ops"}, AgentID: "triage",
			Tool: "github.create_issue",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.key.Validate(); err == nil {
				t.Fatal("Validate accepted a key without the platform-owned prefix")
			}
		})
	}
}

func TestKeyValidate_acceptsCompleteKey(t *testing.T) {
	if err := testKey().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestNilStoreStillValidatesInputs(t *testing.T) {
	var store *dedupe.Postgres
	now := time.Date(2026, 8, 25, 19, 0, 0, 0, time.UTC)

	if _, _, err := store.Lookup(t.Context(), dedupe.Key{}, now); err == nil {
		t.Fatal("Lookup accepted an invalid key because the store was nil")
	}
	if _, err := store.Reserve(t.Context(), testKey(), "", time.Minute, now); err == nil {
		t.Fatal("Reserve accepted an empty run because the store was nil")
	}
	if err := store.Confirm(t.Context(), testKey(), "run-a", 0, time.Hour, now); err == nil {
		t.Fatal("Confirm accepted an empty step because the store was nil")
	}
	if err := store.Release(t.Context(), testKey(), ""); err == nil {
		t.Fatal("Release accepted an empty run because the store was nil")
	}

	rec, err := store.Reserve(t.Context(), testKey(), "run-a", time.Minute, now)
	if err != nil {
		t.Fatalf("Reserve valid input: %v", err)
	}
	if rec.State != dedupe.StateReserved || rec.RunID != "run-a" {
		t.Fatalf("Reserve = %+v, want an inert reservation for valid input", rec)
	}
}
