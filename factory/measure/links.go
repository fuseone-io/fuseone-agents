package main

import (
	"context"
	"fmt"
	"strings"
)

// refs is the item's links connection: the issues a pull request closes, or
// the pull requests that close an issue.
func (raw *rawItem) refs() rawRefs {
	for _, r := range []*rawRefs{raw.Closing, raw.ClosingBy} {
		if r != nil {
			return *r
		}
	}
	return rawRefs{}
}

// addLinks keeps the references in this repository, once each.
func (r reader) addLinks(it *Item, refs rawRefs) {
	for _, ref := range refs.Nodes {
		if r.ours(ref) && !contains(it.Linked, ref.Number) {
			it.Linked = append(it.Linked, ref.Number)
		}
	}
}

// linkPages reads every later page of the items' links, so an issue with
// more pull requests than the first page holds loses none of them.
func (r reader) linkPages(ctx context.Context, s Snapshot, targets []target) error {
	for len(targets) > 0 {
		n := min(batchSize, len(targets))
		next, err := r.linkBatch(ctx, s, targets[:n])
		if err != nil {
			return err
		}
		targets = append(targets[n:], next...)
	}
	return nil
}

func (r reader) linkBatch(ctx context.Context, s Snapshot, batch []target) ([]target, error) {
	doc, vars := linkQuery(batch)
	vars["owner"], vars["name"] = r.owner, r.name
	var res struct {
		Repository map[string]*rawItem `json:"repository"`
	}
	if err := r.c.query(ctx, doc, vars, &res); err != nil {
		return nil, fmt.Errorf("links: %w", err)
	}
	var more []target
	for i, t := range batch {
		raw := res.Repository[fmt.Sprintf("r%d", i)]
		if raw == nil {
			return nil, fmt.Errorf("%w: the links of #%d are not readable", errGitHub, t.number)
		}
		refs := raw.refs()
		r.addLinks(s.Items[t.number], refs)
		if refs.PageInfo.HasNextPage {
			more = append(more, target{number: t.number, isPR: t.isPR, cursor: refs.PageInfo.EndCursor})
		}
	}
	return more, nil
}

func linkQuery(batch []target) (string, map[string]any) {
	vars := map[string]any{}
	params := []string{"$owner: String!", "$name: String!"}
	var b strings.Builder
	for i, t := range batch {
		c := fmt.Sprintf("r%d", i)
		params = append(params, "$"+c+": String")
		vars[c] = t.cursor
		kind, conn, extra := "issue", "closedByPullRequestsReferences", ", includeClosedPrs: true"
		if t.isPR {
			kind, conn, extra = "pullRequest", "closingIssuesReferences", ""
		}
		fmt.Fprintf(&b, "%s: %s(number: %d) { %s(first: 100, after: $%s%s) %s }\n", c, kind, t.number, conn, c, extra, refNodes)
	}
	doc := "query(" + strings.Join(params, ", ") + ") { repository(owner: $owner, name: $name) {\n" + b.String() + "} }"
	return doc, vars
}
