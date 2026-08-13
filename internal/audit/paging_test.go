package audit_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/audit"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Paging the trail.

A limit alone answers "the hundred most recent" and nothing else, which for an
auditor is the same as answering nothing: the entry they came for is almost
never in the most recent hundred. What follows asserts the property that makes
a page worth having — walk the pages and you have walked the trail, each entry
once.
*/
func TestRead_walkedInPages_yieldsEveryEntryExactlyOnce(t *testing.T) {
	reader, pool := readerFor(t)
	for i := range 25 {
		seedDecision(t, pool, noon.Add(time.Duration(i)*time.Minute), 1+i%4, "finance")
	}
	for i := range 12 {
		seedAdmin(t, pool, noon.Add(time.Duration(i)*time.Minute),
			fmt.Sprintf("usr_%d", i), "tool.classified", "finance")
	}

	whole, _, err := reader.Read(t.Context(), audit.Filter{}, 200)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var walked []audit.Entry
	cursor := ""
	for range 20 {
		page, next, err := reader.Read(t.Context(), audit.Filter{Cursor: cursor}, 7)
		if err != nil {
			t.Fatalf("read page: %v", err)
		}
		walked = append(walked, page...)
		if next == "" {
			break
		}
		cursor = next
	}

	if len(walked) != len(whole) {
		t.Fatalf("walking the pages gave %d entries; the trail holds %d", len(walked), len(whole))
	}
	for i := range whole {
		if walked[i].At != whole[i].At || walked[i].Verb != whole[i].Verb ||
			walked[i].RunID != whole[i].RunID || walked[i].Seq != whole[i].Seq {
			t.Fatalf("entry %d differs: paged %+v, whole %+v", i, walked[i], whole[i])
		}
	}
}

// The two records are merged, so a page boundary can fall between them. That
// is the case a cursor built from one position gets wrong.
func TestRead_bothRecordsShareTheInstant_pageBoundaryDropsNeither(t *testing.T) {
	reader, pool := readerFor(t)
	// Every entry at the same instant: the tie is the point.
	for i := range 6 {
		seedDecisionAs(t, pool, fmt.Sprintf("run-tie-%d", i), noon)
		seedAdmin(t, pool, noon, fmt.Sprintf("usr_%d", i), "tool.classified", "finance")
	}

	seen := map[string]int{}
	cursor := ""
	for range 12 {
		page, next, err := reader.Read(t.Context(), audit.Filter{Cursor: cursor}, 3)
		if err != nil {
			t.Fatalf("read page: %v", err)
		}
		for _, e := range page {
			seen[fmt.Sprintf("%s|%s|%s|%d", e.Source, e.Actor, e.RunID, e.Seq)]++
		}
		if next == "" {
			break
		}
		cursor = next
	}

	for key, n := range seen {
		if n != 1 {
			t.Errorf("%s appeared %d times across the pages", key, n)
		}
	}
	if len(seen) != 12 {
		t.Errorf("walked %d distinct entries; seeded 12", len(seen))
	}
}

func TestRead_trailEndsWithinTheLimit_returnsNoCursor(t *testing.T) {
	reader, pool := readerFor(t)
	seedDecision(t, pool, noon, 1, "finance")

	_, next, err := reader.Read(t.Context(), audit.Filter{}, 50)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if next != "" {
		t.Errorf("a trail that fit in one page offered a next page: %q", next)
	}
}

// A cursor is a position in a query, not a bearer token for the whole trail.
// Handing one back with different scopes must not widen what it reaches.
func TestRead_cursorFromAnotherScope_stillCannotReachIt(t *testing.T) {
	reader, pool := readerFor(t)
	for i := range 8 {
		seedDecision(t, pool, noon.Add(time.Duration(i)*time.Minute), 1, "finance")
	}

	_, next, err := reader.Read(t.Context(), audit.Filter{}, 3)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	narrowed := audit.Filter{
		Cursor: next,
		Scopes: []domain.Scope{{Company: "acme", Area: "support"}},
	}
	page, _, err := reader.Read(t.Context(), narrowed, 50)
	if err != nil {
		t.Fatalf("read narrowed: %v", err)
	}
	for _, e := range page {
		if e.Scope.Area == "finance" {
			t.Fatalf("a cursor carried an entry from %s past the scope filter", e.Scope.Area)
		}
	}
}

// seedDecisionAs writes one gate decision under a run the caller names, so
// several can share an instant without sharing a chain.
func seedDecisionAs(t *testing.T, pool *pgxpool.Pool, runID string, at time.Time) {
	t.Helper()
	store := ledger.NewPostgres(pool)
	scope := domain.Scope{Company: "acme", Area: "finance"}
	for _, step := range []domain.Step{
		{RunID: domain.RunID(runID), Kind: domain.StepRunStarted, At: at,
			Scope: scope, AgentID: "triage", VersionID: "v1"},
		{RunID: domain.RunID(runID), Kind: domain.StepGateDecided, At: at,
			Scope: scope, AgentID: "triage", VersionID: "v1",
			Payload: []byte(`{"tool":"crm.reply","verdict":1}`)},
	} {
		if _, err := store.Append(t.Context(), step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}
}
