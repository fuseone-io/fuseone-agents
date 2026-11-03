package main

import (
	"slices"
	"strings"
)

// unmappedRow is an actor that acted in the window and is neither a person
// nor a bot in the map. Its acts are attributed to no role.
type unmappedRow struct {
	Login, Type string
	Seen        string // the kinds of act, comma-separated
	Events      int
}

func kindName(k Kind) string {
	switch k {
	case KindStatus:
		return "board move"
	case KindComment:
		return "comment"
	case KindReopened:
		return "reopen"
	case KindClosed:
		return "close"
	case KindMerged:
		return "merge"
	case KindLabeled, KindUnlabeled:
		return "label"
	case KindReview:
		return "review"
	case KindCommit:
		return "commit"
	case KindDismissed:
		return "review dismissal"
	}
	return "event"
}

// unmappedRows lists every unmapped actor of the window, by login.
func unmappedRows(s Snapshot, bots Bots, w Window) []unmappedRow {
	rows := map[Actor]*unmappedRow{}
	note := func(a Actor, seen string) {
		r := rows[a]
		if r == nil {
			r = &unmappedRow{Login: a.Login, Type: a.Type}
			rows[a] = r
		}
		r.Events++
		if !strings.Contains(", "+r.Seen+", ", ", "+seen+", ") {
			r.Seen = joinSorted(r.Seen, seen)
		}
	}
	for _, it := range s.Items {
		for _, e := range it.Events {
			if w.contains(e.At) && bots.classify(e.Actor) == OwnerUnmapped {
				note(e.Actor, kindName(e.Kind))
			}
		}
		for _, st := range it.Statuses {
			if w.contains(st.At) && bots.classify(st.Creator) == OwnerUnmapped {
				note(st.Creator, "commit status")
			}
		}
	}
	var out []unmappedRow
	for _, r := range rows {
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b unmappedRow) int {
		return strings.Compare(a.Login+"\x00"+a.Type, b.Login+"\x00"+b.Type)
	})
	return out
}

func joinSorted(list, add string) string {
	var parts []string
	if list != "" {
		parts = strings.Split(list, ", ")
	}
	parts = append(parts, add)
	slices.Sort(parts)
	return strings.Join(parts, ", ")
}
