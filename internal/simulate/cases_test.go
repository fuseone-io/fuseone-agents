package simulate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

// A case set is the last N real occurrences of the thing an agent handles.
// Uploaded rather than fetched: the authoring path does not touch production,
// and the author decides what the platform sees.

type fakeStore struct {
	stored map[string][]byte
	seqs   []int64
}

func (f *fakeStore) PutFor(_ context.Context, kind, owner string, seq int64, data []byte) (string, error) {
	if f.stored == nil {
		f.stored = map[string][]byte{}
	}
	ref := kind + "://" + owner + "/" + string(rune('0'+seq))
	f.stored[ref] = data
	f.seqs = append(f.seqs, seq)
	return ref, nil
}

func TestLoad_eachLineBecomesOneCase(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	got, err := simulate.Load(t.Context(), store, domain.AgentID("suporte"), []byte(
		`{"assunto":"cobrança"}`+"\n"+`{"assunto":"acesso"}`+"\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 || len(store.stored) != 2 {
		t.Errorf("got %d cases, stored %d", len(got), len(store.stored))
	}
	// Handed back as well as stored, so whoever runs them now is running the
	// same bytes that were filed.
	if string(got[0]) != `{"assunto":"cobrança"}` {
		t.Errorf("case 1 = %s", got[0])
	}
}

func TestLoad_blankLines_areNotCases(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	got, err := simulate.Load(t.Context(), store, "suporte", []byte("\n\n"+`{"a":1}`+"\n\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// An export ending in a newline is every export. Counting it as a case
	// would put an empty occurrence in a report somebody reads as real.
	if len(got) != 1 {
		t.Errorf("got %d cases, want 1", len(got))
	}
}

func TestLoad_aLineThatIsNotJSON_namesTheLineAndRefusesTheFile(t *testing.T) {
	t.Parallel()

	_, err := simulate.Load(t.Context(), &fakeStore{}, "suporte",
		[]byte(`{"a":1}`+"\nnão é json\n"+`{"b":2}`))

	// Refused whole rather than partly loaded. Fifty cases minus one that
	// nobody was told about is a simulation whose coverage is a lie, and the
	// author can fix an export they were told the line number of.
	if err == nil || !strings.Contains(err.Error(), "2") {
		t.Fatalf("got %v, want the line named", err)
	}
}

func TestLoad_anEmptyFile_isRefused(t *testing.T) {
	t.Parallel()

	if _, err := simulate.Load(t.Context(), &fakeStore{}, "suporte", []byte("  \n")); err == nil {
		t.Error("want a refusal")
	}
}
