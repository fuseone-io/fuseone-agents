package main

import (
	"reflect"
	"strconv"
	"testing"
	"time"
)

func measureOf(t *testing.T, s Snapshot, window, now string) Measures {
	t.Helper()
	w := newWindow(at(t, window), 3, at(t, now))
	return measure(s, testBots(t), Scope{Repo: "acme/widgets", Project: project, Window: w, Now: at(t, now)})
}

func laneRowsOf(m Measures) []string {
	var out []string
	for _, r := range m.Lanes {
		out = append(out, r.Week.Format("2006-01-02")+" "+r.Lane+" n="+strconv.Itoa(r.Stays)+
			" p50="+r.Median.String()+" p90="+r.P90.String())
	}
	return out
}

func TestMeasure_openStays_outOfThePercentilesAndCountedApart(t *testing.T) {
	t.Parallel()
	closed := &Item{Number: 1, OnBoard: true, Events: []Event{
		move(t, "2026-10-05T00:00:00Z", "", "Triagem", person),
		move(t, "2026-10-05T02:00:00Z", "Triagem", "Preparado", triBot),
		move(t, "2026-10-05T03:00:00Z", "Preparado", "Triagem", person),
		move(t, "2026-10-05T07:00:00Z", "Triagem", "Preparado", triBot),
		move(t, "2026-10-05T08:00:00Z", "Preparado", "Done", person),
	}}
	open1 := &Item{Number: 2, OnBoard: true, Events: []Event{move(t, "2026-10-06T00:00:00Z", "", "Triagem", person)}}
	open2 := &Item{Number: 3, OnBoard: true, Events: []Event{move(t, "2026-10-07T00:00:00Z", "", "Triagem", person)}}
	m := measureOf(t, snapshot(closed, open1, open2), "2026-10-05T00:00:00Z", "2026-10-08T06:00:00Z")
	want := []string{
		"2026-10-05 Triagem n=2 p50=2h0m0s p90=4h0m0s",
		"2026-10-05 Preparado n=2 p50=1h0m0s p90=1h0m0s",
	}
	if got := laneRowsOf(m); !reflect.DeepEqual(got, want) {
		t.Errorf("lanes:\n got %q\nwant %q", got, want)
	}
	if want := []openRow{{Lane: "Triagem", Open: 2, Oldest: 54 * time.Hour}}; !reflect.DeepEqual(m.Open, want) {
		t.Errorf("open = %+v, want %+v", m.Open, want)
	}
}

func TestMeasure_stayCountsInTheWeekItEnded(t *testing.T) {
	t.Parallel()
	card := &Item{Number: 1, OnBoard: true, Events: []Event{
		move(t, "2026-09-30T00:00:00Z", "", "Triagem", person),             // before the window
		move(t, "2026-10-05T01:00:00Z", "Triagem", "Preparado", triBot),    // ends in week 1
		move(t, "2026-10-18T23:59:59.999Z", "Preparado", "Doing", person),  // Sunday's last instant: week 2
		move(t, "2026-10-19T00:00:00Z", "Doing", "Travado", qaBot),         // Monday 00:00: week 3
		move(t, "2026-10-26T00:00:00Z", "Travado", "Triagem", person),      // after the window
		move(t, "2026-10-26T01:00:00Z", "Triagem", "Preparado", architect), // after the window
	}}
	m := measureOf(t, snapshot(card), "2026-10-05T00:00:00Z", "2026-11-03T00:00:00Z")
	want := []string{
		"2026-10-19 Doing n=1 p50=1ms p90=1ms",
		"2026-10-12 Preparado n=1 p50=334h59m59.999s p90=334h59m59.999s",
		"2026-10-05 Triagem n=1 p50=121h0m0s p90=121h0m0s",
	}
	if got := laneRowsOf(m); !reflect.DeepEqual(got, want) {
		t.Errorf("lanes:\n got %q\nwant %q", got, want)
	}
}

var architect = archBot

func TestMeasure_movesBelongToWhoMadeThem(t *testing.T) {
	t.Parallel()
	card := &Item{Number: 1, OnBoard: true, Events: []Event{
		move(t, "2026-10-05T00:00:00Z", "", "Triagem", person),
		move(t, "2026-10-05T01:00:00Z", "Triagem", "Preparado", triBot),
		move(t, "2026-10-05T02:00:00Z", "Preparado", "Doing", person),
		move(t, "2026-10-05T03:00:00Z", "Doing", "Travado", strayBot),
	}}
	m := measureOf(t, snapshot(card), "2026-10-05T00:00:00Z", "2026-10-06T00:00:00Z")
	moves := map[Owner]int{}
	for _, r := range m.Roles {
		moves[r.Owner] += r.Moves
	}
	if moves[OwnerPerson] != 2 || moves[OwnerTriage] != 1 || moves[OwnerUnmapped] != 0 {
		t.Errorf("moves = %v, want person 2, triage 1, the unknown bot's in no row", moves)
	}
}

func TestMeasure_unknownBot_reportedUnmappedAndGivenNothing(t *testing.T) {
	t.Parallel()
	card := &Item{Number: 1, OnBoard: true, Events: []Event{
		move(t, "2026-10-05T00:00:00Z", "", "Triagem", person),
		comment(t, "2026-10-05T00:30:00Z", "Triagem", Actor{"Bot", "t-app-old[bot]"}),
		move(t, "2026-10-05T01:00:00Z", "Triagem", "Preparado", Actor{"Bot", "t-app-old[bot]"}),
		move(t, "2026-10-05T02:00:00Z", "Preparado", "Triagem", person),
	}}
	pr := devPR([]Event{review(t, "2026-10-05T03:00:00Z", "CHANGES_REQUESTED", shaA, Actor{"Bot", "r-app-old[bot]"})},
		qaStatus(t, "2026-10-05T04:00:00Z", shaA, "failure", strayBot))
	m := measureOf(t, snapshot(card, pr), "2026-10-05T00:00:00Z", "2026-10-06T00:00:00Z")
	want := []unmappedRow{
		{Login: "r-app-old[bot]", Type: "Bot", Seen: "review", Events: 1},
		{Login: "stray-app[bot]", Type: "Bot", Seen: "commit status", Events: 1},
		{Login: "t-app-old[bot]", Type: "Bot", Seen: "board move, comment", Events: 2},
	}
	if !reflect.DeepEqual(m.Unmapped, want) {
		t.Errorf("unmapped:\n got %+v\nwant %+v", m.Unmapped, want)
	}
	wantSig := []signalRow{{Week: at(t, "2026-10-05T00:00:00Z"), Owner: OwnerUnattributed,
		Kind: returnedKind("Preparado", ruleNoRole), Count: 1}}
	if !reflect.DeepEqual(m.Signals, wantSig) {
		t.Errorf("signals:\n got %+v\nwant %+v", m.Signals, wantSig)
	}
}
