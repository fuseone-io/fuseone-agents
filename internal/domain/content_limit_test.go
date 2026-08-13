package domain_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestTruncate_underTheLimit_isUntouched(t *testing.T) {
	t.Parallel()

	data := []byte(`{"ok":true}`)
	got, cut := domain.Truncate(data, 1024)

	if cut || !bytes.Equal(got, data) {
		t.Errorf("Truncate = %q, %v; want the payload unchanged", got, cut)
	}
}

func TestTruncate_pastTheLimit_saysSoInTheBytesItKeeps(t *testing.T) {
	t.Parallel()

	// The model is the reader that matters. Handed half a JSON document with
	// no notice it would reason over it as though it were whole, which is
	// worse than being told the answer is incomplete.
	got, cut := domain.Truncate(bytes.Repeat([]byte("a"), 4096), 1024)

	if !cut {
		t.Fatal("a payload four times the limit was not reported as truncated")
	}
	if !strings.Contains(string(got), "truncated") {
		t.Errorf("kept %q, want it to say it is partial", string(got[:64]))
	}
	if !strings.Contains(string(got), "4096") {
		t.Error("the notice does not say how much the tool actually returned")
	}
}

func TestTruncate_neverExceedsTheLimitItEnforces(t *testing.T) {
	t.Parallel()

	// A limit the store's own marker could push past is not a limit. Small
	// ones are where that shows: the notice is longer than the room.
	for _, limit := range []int{16, 64, 200, 1024} {
		got, _ := domain.Truncate(bytes.Repeat([]byte("a"), 8192), limit)
		if len(got) > limit {
			t.Errorf("limit %d kept %d bytes", limit, len(got))
		}
	}
}

func TestTruncate_noLimit_keepsEverything(t *testing.T) {
	t.Parallel()

	// Zero is how a caller says "unbounded", and one that never sets a limit
	// gets that rather than an empty payload.
	data := bytes.Repeat([]byte("a"), 4096)
	if got, cut := domain.Truncate(data, 0); cut || len(got) != len(data) {
		t.Errorf("Truncate with no limit kept %d bytes, cut = %v", len(got), cut)
	}
}
