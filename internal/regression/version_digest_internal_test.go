package regression

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// The line a version's digest is made of is a contract with every version already written,
// and with the activation's SQL that wrote the first ones: a case with no world must hash to
// exactly the line it always did, or the first write after worlds arrive records a version
// in which nothing changed. The format is restated here on purpose.
func TestVersionDigest_aCaseWithNoWorld_isTheLineItAlwaysWas(t *testing.T) {
	t.Parallel()
	before := sha256.Sum256([]byte("5:reg-1 in ex\n"))
	got := versionDigest([]versionLine{{CaseID: "reg-1", InputDigest: "in", ExpectationsDigest: "ex"}})
	if got != hex.EncodeToString(before[:]) {
		t.Errorf("digest %s, want the line before worlds", got)
	}
}

func TestVersionDigest_aWorld_changesTheLine(t *testing.T) {
	t.Parallel()
	none := versionDigest([]versionLine{{CaseID: "reg-1", InputDigest: "in", ExpectationsDigest: "ex"}})
	world := versionDigest([]versionLine{{CaseID: "reg-1", InputDigest: "in", ExpectationsDigest: "ex", WorldDigest: "w"}})
	if world == none {
		t.Error("a case with a world hashes like one without")
	}
}
