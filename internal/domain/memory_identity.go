package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// canonicalKeyVersion travels inside the key so the rule below can change
// without every stored key quietly meaning something else. A reader comparing
// two keys of different versions is comparing two different questions, and the
// prefix is what lets them notice.
const canonicalKeyVersion = "v1"

/*
CanonicalIdentityKey is the identity two people teaching the same fact land on.

MemoryAssertionID hashes the strings as typed, which is right for naming a row:
what a memory is called must not change under it. But it means "Slack Alerts"
and " slack   alerts " are two memories that never find each other, and the
duplicate check the console needs cannot be built on it.

So there are two identities, deliberately. The raw id says which row this is.
The canonical key says which fact this is.

The rule is spelling, not meaning: normalise how the characters are encoded,
fold case, collapse the whitespace somebody typed twice. Accents stay — "sessão"
and "sessao" are different words, and folding them together would merge memories
nobody said were the same. Anything looser than this belongs in the similarity
recommendation, which suggests and never merges.
*/
func CanonicalIdentityKey(a MemoryAssertion) string {
	h := sha256.New()
	for _, part := range []string{
		string(a.Scope.Company), string(a.Scope.Area), string(a.AgentID),
		a.Kind, a.Subject, a.Signature,
	} {
		// The NUL is what keeps the fields apart: without it a subject ending
		// where a signature begins would hash the same as the two spelled the
		// other way round. It is an input to the hash and never a byte of the
		// answer — PostgreSQL refuses a zero byte inside text, and the key
		// exists to live in a column.
		writeMemoryPart(h, canonicalField(part))
	}
	return canonicalKeyVersion + ":sha256:" + hex.EncodeToString(h.Sum(nil))
}

// canonicalField is the whole spelling rule, in one place so the key and any
// future reader of it cannot drift apart.
func canonicalField(v string) string {
	// NFC first: the same accented letter arrives precomposed from one keyboard
	// and as a letter plus a combining mark from another, and in Portuguese that
	// is an ordinary Tuesday rather than an edge case.
	v = norm.NFC.String(v)
	// Fields both trims and collapses every run of whitespace, including the
	// tab somebody pasted in the middle.
	v = strings.Join(strings.Fields(v), " ")
	return strings.ToLower(v)
}
