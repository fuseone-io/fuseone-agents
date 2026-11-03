package guard

import (
	"context"
	"fmt"
)

// ReadyPR is one pull request the Merger may act on: its number, the head it
// was judged on, its card, and merge_if_ready's verdict on it right now.
type ReadyPR struct {
	PR      int    `json:"pr"`
	HeadSHA string `json:"head_sha"`
	ItemID  string `json:"item_id"`
	Ready   bool   `json:"ready"`
	Reason  Reason `json:"reason,omitempty"`
	Check   string `json:"check,omitempty"`
}

// ReadySkipped counts the labelled pull requests left out, and why.
type ReadySkipped struct {
	NoCard     int `json:"no_card"`
	ManyCards  int `json:"many_cards"`
	NotInDoing int `json:"not_in_doing"`
}

// ReadyList is list_ready_prs' answer: numbers, SHAs, item ids and closed
// reasons only. It is the Merger's one read before its claim, so nothing in
// it may be text a person or GitHub wrote.
type ReadyList struct {
	PRs     []ReadyPR    `json:"prs"`
	Skipped ReadySkipped `json:"skipped"`
}

// ReadyLister answers list_ready_prs.
type ReadyLister struct {
	gh     *Client
	board  *Board
	merger *Merger
	base   string
}

// NewReadyLister builds the listing from the Merger instance's configuration.
func NewReadyLister(cfg Config, gh *Client) *ReadyLister {
	return &ReadyLister{gh: gh, board: NewBoard(cfg.Board, RoleMerger, gh), merger: NewMerger(cfg, gh), base: cfg.Base}
}

// List returns the open pull requests on the base labelled ready-to-merge
// whose one card is in Doing, oldest first, each pre-judged exactly as
// merge_if_ready would judge it now. A card in any other lane is no
// Merger's: Travado is a person's, and the label alone never makes it so.
func (l *ReadyLister) List(ctx context.Context) (ReadyList, error) {
	out := ReadyList{PRs: []ReadyPR{}}
	nodes, err := l.gh.readyPullRequests(ctx, l.base)
	if err != nil {
		return out, fmt.Errorf("list_ready_prs: %w", err)
	}
	for _, n := range nodes {
		cards := l.cardsOf(n)
		switch {
		case len(cards) == 0:
			out.Skipped.NoCard++
			continue
		case len(cards) > 1:
			out.Skipped.ManyCards++
			continue
		}
		pr, inDoing, err := l.judge(ctx, n, cards[0])
		if err != nil {
			return out, fmt.Errorf("list_ready_prs: %w", err)
		}
		if !inDoing {
			out.Skipped.NotInDoing++
			continue
		}
		out.PRs = append(out.PRs, pr)
	}
	return out, nil
}

// cardsOf is the distinct, unarchived items of the configured project on the
// pull request itself and on the issues it closes.
func (l *ReadyLister) cardsOf(n readyNode) []string {
	var cards []string
	add := func(c itemConn) {
		for _, it := range c.Nodes {
			if it.Project.ID == l.board.cfg.ProjectID && !it.IsArchived && validItemID(it.ID) && !contains(cards, it.ID) {
				cards = append(cards, it.ID)
			}
		}
	}
	add(n.ProjectItems)
	for _, issue := range n.Closing.Nodes {
		add(issue.ProjectItems)
	}
	return cards
}

// judge reads the card's lane and, for a card in Doing, pre-judges the merge.
func (l *ReadyLister) judge(ctx context.Context, n readyNode, card string) (ReadyPR, bool, error) {
	item, err := l.gh.readItem(ctx, card)
	if err != nil {
		return ReadyPR{}, false, err
	}
	if lane, reason := l.board.judgeItem(item); reason != "" || lane != LaneDoing {
		return ReadyPR{}, false, nil
	}
	reason, check, err := l.merger.prejudge(ctx, n.Number, n.HeadRefOid)
	if err != nil {
		return ReadyPR{}, false, err
	}
	return ReadyPR{PR: n.Number, HeadSHA: n.HeadRefOid, ItemID: card, Ready: reason == "", Reason: reason, Check: check}, true, nil
}
