package export_test

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/export"
)

// An export is worth signing only if somebody outside this installation can
// check it without trusting us. Every assertion here is a way of tampering.

func chain(t *testing.T, count int) []domain.Step {
	t.Helper()

	var (
		steps []domain.Step
		prev  *domain.Step
	)
	at := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	for i := range count {
		sealed, err := domain.NewStep(prev, domain.Step{
			RunID: "run-1", Kind: domain.StepPlanned,
			Scope:   domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "suporte", VersionID: "v1",
			At:      at.Add(time.Duration(i) * time.Second),
			Payload: []byte(`{"node":"Responder"}`),
		})
		if err != nil {
			t.Fatalf("NewStep: %v", err)
		}
		steps = append(steps, sealed)
		prev = &steps[len(steps)-1]
	}
	return steps
}

func signer(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return key
}

func TestBundle_survivesTheRoundTripItWillActuallyTake(t *testing.T) {
	t.Parallel()

	bundle, err := export.Build("acme", chain(t, 4), signer(t))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	encoded, err := bundle.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Written to a file, mailed, and opened somewhere else. If a timestamp or
	// a payload reshapes on the way, every hash stops matching and the export
	// is worthless exactly when somebody needs it.
	decoded, err := export.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if err := export.Verify(decoded); err != nil {
		t.Errorf("Verify after a round trip: %v", err)
	}
}

func TestVerify_aChangedPayload_isCaught(t *testing.T) {
	t.Parallel()

	bundle, _ := export.Build("acme", chain(t, 3), signer(t))
	bundle.Steps[1].Payload = []byte(`{"node":"Outro"}`)

	// The hash commits to every field of the step, so editing one and leaving
	// the hash is the naive tamper and must not survive.
	if err := export.Verify(bundle); err == nil {
		t.Fatal("an edited payload verified")
	}
}

func TestVerify_aRemovedStep_isCaught(t *testing.T) {
	t.Parallel()

	bundle, _ := export.Build("acme", chain(t, 4), signer(t))
	bundle.Steps = append(bundle.Steps[:1], bundle.Steps[2:]...)

	// Removing an inconvenient step from the middle is the tamper that
	// matters: it leaves every remaining step internally valid, and only the
	// links between them give it away.
	if err := export.Verify(bundle); err == nil {
		t.Fatal("a truncated chain verified")
	}
}

func TestVerify_aResignedBundle_isCaughtWithoutTheRightKey(t *testing.T) {
	t.Parallel()

	steps := chain(t, 3)
	bundle, _ := export.Build("acme", steps, signer(t))

	// Somebody rebuilding the export with their own key produces something
	// internally consistent. What stops it is the reader knowing which public
	// key this installation publishes, so the key travels with the bundle and
	// is what they compare.
	forged, _ := export.Build("acme", steps, signer(t))
	if string(forged.PublicKey) == string(bundle.PublicKey) {
		t.Fatal("two keys collided")
	}
	if err := export.Verify(forged); err != nil {
		t.Errorf("the forgery is internally valid, as expected: %v", err)
	}
	if bundle.Fingerprint() == forged.Fingerprint() {
		t.Error("two different keys share a fingerprint")
	}
}

func TestVerify_aTamperedSignature_isCaught(t *testing.T) {
	t.Parallel()

	bundle, _ := export.Build("acme", chain(t, 3), signer(t))
	bundle.Signature[0] ^= 0xff

	if err := export.Verify(bundle); err == nil {
		t.Fatal("a broken signature verified")
	}
}

func TestBuild_refusesAnEmptyRange(t *testing.T) {
	t.Parallel()

	// A signed statement that nothing happened is a statement somebody can
	// wave about. If there is nothing to export, say so rather than sign it.
	if _, err := export.Build("acme", nil, signer(t)); err == nil {
		t.Fatal("an empty export was signed")
	}
}

func TestBundle_isReadableByThePersonCheckingIt(t *testing.T) {
	t.Parallel()

	bundle, _ := export.Build("acme", chain(t, 2), signer(t))
	encoded, err := bundle.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("the export is not JSON: %v", err)
	}
	steps, _ := document["steps"].([]any)
	first, _ := steps[0].(map[string]any)

	// An export exists to be read. A payload in base64 and fields named after
	// a Go struct make a document somebody has to write a tool to open, which
	// is the opposite of what AU-12 is for.
	if _, ok := first["payload"].(map[string]any); !ok {
		t.Errorf("payload = %#v, want it readable", first["payload"])
	}
	for _, want := range []string{"run", "seq", "kind", "at", "hash"} {
		if _, ok := first[want]; !ok {
			t.Errorf("the export has no %q field: %v", want, keysOf(first))
		}
	}
	// Hex, like every hash this product shows a person anywhere else.
	if hash, _ := first["hash"].(string); len(hash) != 64 {
		t.Errorf("hash = %q, want hex", hash)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestVerify_aRemovedStep_saysWhatIsActuallyWrong(t *testing.T) {
	t.Parallel()

	bundle, _ := export.Build("acme", chain(t, 5), signer(t))
	bundle.Steps = append(bundle.Steps[:2], bundle.Steps[3:]...)

	err := export.Verify(bundle)
	if err == nil {
		t.Fatal("a gapped chain verified")
	}
	// The beginning of the chain is fine. Saying "must start at 1" sends
	// whoever reads it to the wrong end of the file.
	if !errors.Is(err, domain.ErrSeqGap) {
		t.Errorf("err = %v, want it to name the gap", err)
	}
}
