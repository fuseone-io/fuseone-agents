package e2e_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/export"
)

/*
An auditor's whole interaction with this product is: take the file, run
`agentd verify`, believe or do not believe it.

The export package tests that thoroughly against steps it builds itself. What
nothing covered is the seam — a bundle built from a run this platform actually
executed. The export has its own step shape (lowercase JSON, payload as raw
JSON, hex hashes) and re-canonicalises payloads on decode, so a run whose steps
came out of the ledger rather than out of a test helper is where a mismatch
between the two shapes would show up, and only there.
*/
func TestExport_ofARunThisPlatformActuallyRan_verifies(t *testing.T) {
	eachLedger(t, "a real run exports to a bundle that checks out", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, readThenAnswer)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.lookup", Effect: domain.EffectRead,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.open(t, "run-export-1")
		p.settle(t, "run-export-1")

		_, key, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		steps := p.steps(t, "run-export-1")

		bundle, err := export.Build("acme", steps, key)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}

		// Through the bytes, because that is the journey the file takes: the
		// auditor has a document, not a struct.
		encoded, err := bundle.Encode()
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		decoded, err := export.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if err := export.Verify(decoded); err != nil {
			t.Fatalf("Verify a genuine export of a real run: %v", err)
		}

		// And the other half: the same document, one payload edited. An export
		// that could not tell the difference would be worth nothing.
		tampered, err := export.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if len(tampered.Steps) < 2 {
			t.Fatalf("the run produced %d steps, too few to tamper with", len(tampered.Steps))
		}
		tampered.Steps[1].Payload = []byte(`{"node":"algo que ninguém propôs"}`)
		if err := export.Verify(tampered); err == nil {
			t.Error("Verify accepted a bundle whose payload was edited")
		}
	})
}
