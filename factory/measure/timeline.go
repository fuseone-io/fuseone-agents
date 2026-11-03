package main

import (
	"context"
	"fmt"
	"strings"
)

// batchSize is how many timelines one query asks for, each as its own alias
// with first: 100, GitHub's page ceiling.
const batchSize = 20

const (
	issueTypes = "PROJECT_V2_ITEM_STATUS_CHANGED_EVENT, ISSUE_COMMENT, REOPENED_EVENT, CLOSED_EVENT, LABELED_EVENT, UNLABELED_EVENT"
	prTypes    = issueTypes + ", MERGED_EVENT, PULL_REQUEST_REVIEW, REVIEW_DISMISSED_EVENT, PULL_REQUEST_COMMIT"

	actorSel    = `{ __typename login }`
	issueFields = `__typename
... on ProjectV2ItemStatusChangedEvent { createdAt previousStatus status project { number } actor ` + actorSel + ` }
... on IssueComment { createdAt author ` + actorSel + ` body }
... on ReopenedEvent { createdAt actor ` + actorSel + ` }
... on ClosedEvent { createdAt actor ` + actorSel + ` }
... on LabeledEvent { createdAt actor ` + actorSel + ` label { name } }
... on UnlabeledEvent { createdAt actor ` + actorSel + ` label { name } }`
	prFields = issueFields + `
... on MergedEvent { createdAt actor ` + actorSel + ` }
... on PullRequestReview { id submittedAt state author ` + actorSel + ` commit { oid } }
... on ReviewDismissedEvent { createdAt previousReviewState actor ` + actorSel + ` review { id } }
... on PullRequestCommit { commit { oid committedDate author { email user { login } } } }`

	refSel    = `(first: 10) { nodes { number repository { nameWithOwner } } }`
	issueMeta = `closedByPullRequestsReferences(first: 10, includeClosedPrs: true) { nodes { number repository { nameWithOwner } } }`
	prMeta    = `merged author ` + actorSel + ` closingIssuesReferences` + refSel
)

// target is a timeline still to read: from its start, or from cursor.
type target struct {
	number int
	isPR   bool
	cursor string
}

// rawItem is one alias's answer: a page of the timeline and, for the first
// page, the links.
type rawItem struct {
	TimelineItems struct {
		PageInfo pageInfo  `json:"pageInfo"`
		Nodes    []rawNode `json:"nodes"`
	} `json:"timelineItems"`
	Merged    bool      `json:"merged"`
	Author    *rawActor `json:"author"`
	Closing   *rawRefs  `json:"closingIssuesReferences"`
	ClosingBy *rawRefs  `json:"closedByPullRequestsReferences"`
}

type rawRefs struct {
	Nodes []rawRef `json:"nodes"`
}

// timelines reads each target's whole timeline into s, in batches of
// aliases, following every alias's cursor until its last page.
func (r reader) timelines(ctx context.Context, s Snapshot, targets []target) error {
	for len(targets) > 0 {
		n := min(batchSize, len(targets))
		next, err := r.timelineBatch(ctx, s, targets[:n])
		if err != nil {
			return err
		}
		targets = append(targets[n:], next...)
	}
	return nil
}

func (r reader) timelineBatch(ctx context.Context, s Snapshot, batch []target) ([]target, error) {
	doc, vars := timelineQuery(batch)
	vars["owner"], vars["name"] = r.owner, r.name
	var res struct {
		Repository map[string]*rawItem `json:"repository"`
	}
	if err := r.c.query(ctx, doc, vars, &res); err != nil {
		return nil, fmt.Errorf("timelines: %w", err)
	}
	var more []target
	for i, t := range batch {
		raw := res.Repository[fmt.Sprintf("t%d", i)]
		if raw == nil {
			return nil, fmt.Errorf("%w: #%d is not readable", errGitHub, t.number)
		}
		r.absorb(s, t, raw)
		if p := raw.TimelineItems.PageInfo; p.HasNextPage {
			more = append(more, target{number: t.number, isPR: t.isPR, cursor: p.EndCursor})
		}
	}
	return more, nil
}

func timelineQuery(batch []target) (string, map[string]any) {
	vars := map[string]any{}
	params := []string{"$owner: String!", "$name: String!"}
	var b strings.Builder
	for i, t := range batch {
		c := fmt.Sprintf("c%d", i)
		params = append(params, "$"+c+": String")
		vars[c] = nil
		if t.cursor != "" {
			vars[c] = t.cursor
		}
		kind, types, fields, meta := "issue", issueTypes, issueFields, issueMeta
		if t.isPR {
			kind, types, fields, meta = "pullRequest", prTypes, prFields, prMeta
		}
		fmt.Fprintf(&b, "t%d: %s(number: %d) { timelineItems(first: 100, after: $%s, itemTypes: [%s]) "+
			"{ pageInfo { hasNextPage endCursor } nodes { %s } } %s }\n", i, kind, t.number, c, types, fields, meta)
	}
	doc := "query(" + strings.Join(params, ", ") + ") { repository(owner: $owner, name: $name) {\n" + b.String() + "} }"
	return doc, vars
}

// absorb adds one page to the item, creating it on its first page.
func (r reader) absorb(s Snapshot, t target, raw *rawItem) {
	it := s.Items[t.number]
	if it == nil {
		it = &Item{Number: t.number, IsPR: t.isPR, Merged: raw.Merged, Author: raw.Author.actor()}
		for _, refs := range []*rawRefs{raw.Closing, raw.ClosingBy} {
			for _, ref := range refsOf(refs) {
				if r.ours(ref) && !contains(it.Linked, ref.Number) {
					it.Linked = append(it.Linked, ref.Number)
				}
			}
		}
		s.Items[t.number] = it
	}
	for _, n := range raw.TimelineItems.Nodes {
		if e, ok := n.event(); ok {
			it.Events = append(it.Events, e)
		}
	}
}

func refsOf(r *rawRefs) []rawRef {
	if r == nil {
		return nil
	}
	return r.Nodes
}
