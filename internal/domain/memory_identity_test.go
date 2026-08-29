package domain_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestDerivedMemoryIdentity_usesOneStableHumanKey(t *testing.T) {
	t.Parallel()
	kind, signature := domain.DerivedMemoryIdentity("  Alertas do Superset  ")
	if kind != domain.MemoryKindFact || signature != "Alertas do Superset" {
		t.Fatalf("identity = %q/%q, want fact and the trimmed subject", kind, signature)
	}
}

// identity builds an assertion carrying only the six fields identity is made of.
func identity(company, area, agent, kind, subject, signature string) domain.MemoryAssertion {
	return domain.MemoryAssertion{
		Scope:     domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)},
		AgentID:   domain.AgentID(agent),
		Kind:      kind,
		Subject:   subject,
		Signature: signature,
	}
}

/*
The canonical key is what makes two people teaching the same fact land on one
memory.

MemoryAssertionID hashes the raw strings, so "Slack Alerts" and " slack
alerts " are different memories that never find each other. The canonical key
is the identity a duplicate check can trust; the raw id stays what a row is
called.
*/
func TestCanonicalIdentityKey_spacingAndCaseDoNotChangeIt(t *testing.T) {
	t.Parallel()

	got := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "  Slack   Alerts ", "SLACK.not_in_channel"))
	want := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "slack alerts", "slack.not_in_channel"))

	if got != want {
		t.Errorf("key = %q, want the same key as the tidy spelling %q", got, want)
	}
}

// The reason x/text is worth an exception to the stdlib-only rule: in
// Portuguese the same word arrives spelled two ways, and a memory about
// "produção" must not split in two because one of them came from a paste.
func TestCanonicalIdentityKey_composedAndDecomposedAccentsAgree(t *testing.T) {
	t.Parallel()

	composed := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "produção", "deploy"))
	decomposed := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "produção", "deploy"))

	if composed != decomposed {
		t.Errorf("composed = %q, decomposed = %q, want one key", composed, decomposed)
	}
}

// Accents are normalised, never removed: "sessão" and "sessao" are different
// words, and folding them together would merge memories nobody said were the
// same.
func TestCanonicalIdentityKey_keepsAccents(t *testing.T) {
	t.Parallel()

	accented := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "sessão", "auth"))
	plain := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "sessao", "auth"))

	if accented == plain {
		t.Error("accented and unaccented subjects share a key, want them apart")
	}
}

// The version travels in the key so the rule can change without every stored
// key silently meaning something else.
func TestCanonicalIdentityKey_carriesItsVersion(t *testing.T) {
	t.Parallel()

	got := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "slack alerts", "not_in_channel"))

	if !strings.HasPrefix(got, "v1:sha256:") {
		t.Fatalf("key = %q, want the v1:sha256: prefix", got)
	}
	digest := strings.TrimPrefix(got, "v1:sha256:")
	if len(digest) != 64 {
		t.Errorf("digest is %d chars, want the whole sha256", len(digest))
	}
	// Length alone would accept 64 of anything. The contract is hex, because a
	// reader comparing keys by eye and a column indexing them both depend on it.
	if _, err := hex.DecodeString(digest); err != nil {
		t.Errorf("digest %q is not hexadecimal: %v", digest, err)
	}
}

/*
The NUL separating the fields is an input to the hash, never a byte of the key.

PostgreSQL refuses a zero byte inside text, so a key that carried the separator
through would be unstorable in the column it exists for.
*/
func TestCanonicalIdentityKey_holdsNoSeparatorByte(t *testing.T) {
	t.Parallel()

	got := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "", "incident", "slack alerts", "not_in_channel"))

	if strings.ContainsRune(got, 0) {
		t.Error("key carries a NUL byte, which PostgreSQL will not store in text")
	}
}

// Two fields must not run together: a subject ending where a signature begins
// would otherwise be the same key as the two spelled the other way round.
func TestCanonicalIdentityKey_fieldsDoNotBleedIntoEachOther(t *testing.T) {
	t.Parallel()

	first := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "slack", "alerts"))
	second := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "slackalerts", ""))

	if first == second {
		t.Error("subject+signature collided with their concatenation")
	}
}

// The shared namespace is a different identity from any agent's, and the key
// has to say so — it is the whole reason a shared memory does not cover an
// agent's and the other way round.
func TestCanonicalIdentityKey_sharedNamespaceDiffersFromAnAgents(t *testing.T) {
	t.Parallel()

	shared := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "", "incident", "slack alerts", "not_in_channel"))
	scoped := domain.CanonicalIdentityKey(identity(
		"acme", "ops", "triage", "incident", "slack alerts", "not_in_channel"))

	if shared == scoped {
		t.Error("shared and agent-scoped identities share a key")
	}
}
