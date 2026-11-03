package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

// check is one canary premise and what was found.
type check struct {
	name   string
	pass   bool
	detail string
}

// canary checks the two GitHub premises the measures rest on, against an
// issue whose card a role's App moved, with the token the report will use:
//
//  1. the token reads ProjectV2ItemStatusChangedEvent on issue timelines,
//     with the project it belongs to;
//  2. a move an App made (move_card) appears as a Bot actor whose login is
//     that App's bot in the map.
//
// With a pull request it also checks that pull request cards carry the same
// event in their own timeline.
func canary(ctx context.Context, r reader, issue, pr int) []check {
	s := Snapshot{Items: map[int]*Item{}}
	targets := []target{{number: issue}}
	if pr > 0 {
		targets = append(targets, target{number: pr, isPR: true})
	}
	if err := r.timelines(ctx, s, targets); err != nil {
		return []check{{name: premiseRead, detail: "the query failed: " + err.Error()},
			{name: premiseBot, detail: "not checked: nothing was read"}}
	}
	out := []check{readCheck(s.Items[issue], r.project), botCheck(s.Items[issue], r)}
	if pr > 0 {
		c := readCheck(s.Items[pr], r.project)
		c.name = premisePR
		out = append(out, c)
	}
	return out
}

const (
	premiseRead = "the token reads ProjectV2ItemStatusChangedEvent on an issue timeline"
	premiseBot  = "a move by a role's App appears as a Bot with the App's login"
	premisePR   = "a pull request card carries the same event in its own timeline"
)

func readCheck(it *Item, project int) check {
	c := check{name: premiseRead}
	all, ours := 0, len(statusMoves(it, project))
	for _, e := range it.Events {
		if e.Kind == KindStatus {
			all++
		}
	}
	switch {
	case ours > 0:
		c.pass, c.detail = true, fmt.Sprintf("%d status changes on #%d in project %d", ours, it.Number, project)
	case all > 0:
		c.detail = fmt.Sprintf("%d status changes on #%d, none naming project %d: the token cannot read the project", all, it.Number, project)
	default:
		c.detail = fmt.Sprintf("no status change on #%d: name an item whose card was moved", it.Number)
	}
	return c
}

func botCheck(it *Item, r reader) check {
	c := check{name: premiseBot}
	seen := map[string]int{}
	for _, e := range statusMoves(it, r.project) {
		who := r.bots.classify(e.Actor)
		if who.isRole() {
			c.pass, c.detail = true, fmt.Sprintf("#%d was moved by %s, the %s bot", it.Number, e.Actor.Login, who)
			return c
		}
		seen[fmt.Sprintf("%s %s (%s)", orDash(e.Actor.Type), orDash(e.Actor.Login), who)]++
	}
	var parts []string
	for k, n := range seen {
		parts = append(parts, fmt.Sprintf("%s x%d", k, n))
	}
	sort.Strings(parts)
	c.detail = fmt.Sprintf("no move on #%d by a mapped App bot; moves by: %s", it.Number, orDash(strings.Join(parts, "; ")))
	return c
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// printCanary prints the checks and says whether the report can be relied
// on; it returns whether every check passed.
func printCanary(w io.Writer, checks []check, bots Bots) bool {
	ok := true
	for i, c := range checks {
		verdict := "PASS"
		if !c.pass {
			verdict, ok = "FAIL", false
		}
		_, _ = fmt.Fprintf(w, "%s  %d. %s: %s\n", verdict, i+1, c.name, c.detail)
	}
	if p := bots.placeholders(); len(p) > 0 {
		_, _ = fmt.Fprintf(w, "NOTE  bots.yaml still has placeholders for %s; report refuses to run until they are filled in\n",
			strings.Join(p, ", "))
	}
	if ok {
		_, _ = fmt.Fprintln(w, "Result: PASS.")
	} else {
		_, _ = fmt.Fprintln(w, "Result: FAIL. Do not rely on factory-measure report until every check passes.")
	}
	return ok
}
