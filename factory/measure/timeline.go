package main

import (
	"context"
	"fmt"
	"strings"
	"time"
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

	// The first page of each item's links rides along with its first
	// timeline page; links.go reads the rest.
	refNodes  = `{ pageInfo { hasNextPage endCursor } nodes { number repository { nameWithOwner } } }`
	issueMeta = `closedByPullRequestsReferences(first: 10, includeClosedPrs: true) ` + refNodes
	prMeta    = `merged createdAt author ` + actorSel + ` closingIssuesReferences(first: 10) ` + refNodes
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
	CreatedAt time.Time `json:"createdAt"`
	Author    *rawActor `json:"author"`
	Closing   *rawRefs  `json:"closingIssuesReferences"`
	ClosingBy *rawRefs  `json:"closedByPullRequestsReferences"`
}

type rawRefs struct {
	PageInfo pageInfo `json:"pageInfo"`
	Nodes    []rawRef `json:"nodes"`
}

// timelines reads each target's whole timeline into s, in batches of
// aliases, following every alias's cursor until its last page, and then every
// page of each item's links.
func (r reader) timelines(ctx context.Context, s Snapshot, targets []target) error {
	var links []target
	for len(targets) > 0 {
		n := min(batchSize, len(targets))
		next, moreLinks, err := r.timelineBatch(ctx, s, targets[:n])
		if err != nil {
			return err
		}
		targets = append(targets[n:], next...)
		links = append(links, moreLinks...)
	}
	return r.linkPages(ctx, s, links)
}

func (r reader) timelineBatch(ctx context.Context, s Snapshot, batch []target) (more, links []target, err error) {
	doc, vars := timelineQuery(batch)
	vars["owner"], vars["name"] = r.owner, r.name
	var res struct {
		Repository map[string]*rawItem `json:"repository"`
	}
	if err := r.c.query(ctx, doc, vars, &res); err != nil {
		return nil, nil, fmt.Errorf("timelines: %w", err)
	}
	for i, t := range batch {
		raw := res.Repository[fmt.Sprintf("t%d", i)]
		if raw == nil {
			return nil, nil, fmt.Errorf("%w: #%d is not readable", errGitHub, t.number)
		}
		if link, ok := r.absorb(s, t, raw); ok {
			links = append(links, link)
		}
		if p := raw.TimelineItems.PageInfo; p.HasNextPage {
			more = append(more, target{number: t.number, isPR: t.isPR, cursor: p.EndCursor})
		}
	}
	return more, links, nil
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

// absorb adds one page to the item, creating it on its first page. On that
// page it also takes the first page of links, and says where the next one
// starts when there is one.
func (r reader) absorb(s Snapshot, t target, raw *rawItem) (target, bool) {
	var next target
	more := false
	it := s.Items[t.number]
	if it == nil {
		it = &Item{Number: t.number, IsPR: t.isPR, Merged: raw.Merged, Created: raw.CreatedAt, Author: raw.Author.actor()}
		s.Items[t.number] = it
		refs := raw.refs()
		r.addLinks(it, refs)
		if more = refs.PageInfo.HasNextPage; more {
			next = target{number: t.number, isPR: t.isPR, cursor: refs.PageInfo.EndCursor}
		}
	}
	for _, n := range raw.TimelineItems.Nodes {
		if e, ok := n.event(); ok {
			it.Events = append(it.Events, e)
		}
	}
	return next, more
}
