package main

import (
	"fmt"
	"time"
)

// How a return to Triagem found its role.
const (
	ruleReturnTo  = "by the last Return to: comment"
	ruleLastWrite = "by the last role write"
	ruleNoRole    = "no role wrote before it"
)

func returnedKind(from, rule string) string {
	return fmt.Sprintf("returned to Triagem by a person from %s (%s)", from, rule)
}

// returns: a person moved the card into Triagem from another lane. The
// signal goes to the role responsible for the card's last block, not to the
// role that owns Triagem:
//
//  1. the role whose bot wrote the most recent comment ending in a
//     "Return to: <lane>" line before the move, on the card or a pull
//     request linked to it (the Travado protocol, factory/LANES.md);
//  2. failing that, the role whose bot wrote last before the move — a
//     comment, a review, a commit, a label or a board move;
//  3. failing that, nobody: unattributed.
//
// The from lane is kept in the signal, so a return out of Travado, which is
// the protocol working, can be told apart from one out of Preparado or Doing.
func returns(s Snapshot, card *Item, bots Bots, project int) []Signal {
	var out []Signal
	for _, m := range statusMoves(card, project) {
		back := m.To == LaneTriagem && m.From != "" && m.From != LaneTriagem
		if !back || bots.classify(m.Actor) != OwnerPerson {
			continue
		}
		owner, rule := lastBlock(s, card, bots, m.At)
		out = append(out, Signal{owner, returnedKind(m.From, rule), m.At, fmt.Sprintf("#%d", card.Number)})
	}
	return out
}

func lastBlock(s Snapshot, card *Item, bots Bots, before time.Time) (Owner, string) {
	var block, write Event
	var blockBy, writeBy Owner
	for _, it := range blockScope(s, card) {
		for _, e := range it.Events {
			who := bots.classify(e.Actor)
			if !who.isRole() || !e.At.Before(before) {
				continue
			}
			if e.Kind == KindComment && e.ReturnTo != "" && !e.At.Before(block.At) {
				block, blockBy = e, who
			}
			if !e.At.Before(write.At) {
				write, writeBy = e, who
			}
		}
	}
	switch {
	case blockBy != "":
		return blockBy, ruleReturnTo
	case writeBy != "":
		return writeBy, ruleLastWrite
	}
	return OwnerUnattributed, ruleNoRole
}

// blockScope is where a role writes about a card: the card, its linked pull
// requests, and, for a pull request card, the issues it closes.
func blockScope(s Snapshot, card *Item) []*Item {
	scope := append([]*Item{card}, linkedPRs(s, card)...)
	for _, n := range card.Linked {
		if it := s.Items[n]; it != nil && !it.IsPR {
			scope = append(scope, it)
		}
	}
	return scope
}
