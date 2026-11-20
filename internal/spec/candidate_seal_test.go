package spec_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/spec"
)

/*
One seal, by its id, as the experiments form names what it will run (NT-018 phase 2, 2d):
found whatever page of the listing it would fall on, under the same rule as the listing —
a seal in a scope the caller may publish in, of a version not yet published — and nothing
else, so a link to a seal says nothing about one the caller may not see.
*/
func TestCandidates_aSealByItsID_pastTheListingsLimit(t *testing.T) {
	w := newCandidateWorld(t)
	t0 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	oldest, version := w.seal(t, "cx", t0)
	for i := 1; i <= spec.CandidateListLimit; i++ {
		w.seal(t, "cx", t0.Add(time.Duration(i)*time.Second))
	}

	got, err := w.reader.Seal(context.Background(), w.agent, oldest, allowAreas("cx"))
	if err != nil || got.SealID != oldest || got.Version != version {
		t.Fatalf("Seal(oldest) = %+v (%v), want the seal past the listing's page", got, err)
	}
}

func TestCandidates_aSealByItsID_refusals(t *testing.T) {
	w := newCandidateWorld(t)
	t0 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	cx, _ := w.seal(t, "cx", t0)
	sales, _ := w.seal(t, "sales", t0.Add(time.Minute))
	published, err := spec.Parse("c.agent.md", []byte(fmt.Sprintf(candidateText, w.agent, "sales")))
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.NewRegistry(w.pool).Publish(context.Background(), published, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	other := newCandidateWorld(t)
	for name, tc := range map[string]struct {
		reader candidateWorld
		seal   string
		areas  []string
	}{
		"outside the sealed scope":   {w, cx, []string{"finance"}},
		"of a version now published": {w, sales, []string{"sales"}},
		"of another agent":           {other, cx, []string{"cx"}},
		"not a seal id":              {w, "not-a-uuid", []string{"cx"}},
		"a seal nobody wrote":        {w, "00000000-0000-0000-0000-000000000000", []string{"cx"}},
	} {
		if _, err := tc.reader.reader.Seal(context.Background(), tc.reader.agent, tc.seal, allowAreas(tc.areas...)); !errors.Is(err, spec.ErrCandidateNotFound) {
			t.Errorf("%s: err = %v, want ErrCandidateNotFound", name, err)
		}
	}
}

// A seal names the draft it was sealed from, so a rehearsal of it can be tied to its
// author's conversation on that draft (NT-018 phase 4) by the server, never by the client.
func TestCandidates_aSealByItsID_namesTheDraftItWasSealedFrom(t *testing.T) {
	w := newCandidateWorld(t)
	seal, _ := w.seal(t, "cx", time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC))
	var draft string
	if err := w.pool.QueryRow(context.Background(), `select draft_id::text from agent_seals where seal_id = $1::uuid`, seal).Scan(&draft); err != nil {
		t.Fatal(err)
	}

	got, err := w.reader.Seal(context.Background(), w.agent, seal, allowAreas("cx"))
	if err != nil || got.DraftID != draft {
		t.Fatalf("Seal = %+v (%v), want draft %s", got, err, draft)
	}
}

// The opener's resolution names the draft too: a rehearsal belongs to its author's
// conversation on it.
func TestCandidateReader_aSealForTheOpener_namesTheDraftItWasSealedFrom(t *testing.T) {
	w := newCandidateWorld(t)
	seal, _ := w.seal(t, "cx", time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC))
	var draft string
	if err := w.pool.QueryRow(context.Background(), `select draft_id::text from agent_seals where seal_id = $1::uuid`, seal).Scan(&draft); err != nil {
		t.Fatal(err)
	}

	got, found, err := w.reader.Candidate(context.Background(), w.agent, seal)
	if err != nil || !found || got.DraftID != draft {
		t.Fatalf("Candidate = %+v (%v, %v), want draft %s", got, found, err, draft)
	}
}
