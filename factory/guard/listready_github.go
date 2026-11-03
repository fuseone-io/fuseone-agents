package guard

import (
	"context"
	"fmt"
	"net/http"
)

// readyQuery lists the open pull requests on the protected base carrying
// factory:ready-to-merge, oldest first, with the project items of each and of
// the issues it closes. It asks for numbers, SHAs and ids only: no title, no
// body, no label text. The label and the base travel as variables.
const readyQuery = `query($owner: String!, $name: String!, $base: String!, $label: String!) { ` +
	`repository(owner: $owner, name: $name) { pullRequests(states: OPEN, baseRefName: $base, ` +
	`labels: [$label], first: 50, orderBy: {field: CREATED_AT, direction: ASC}) { ` +
	`pageInfo { hasNextPage } nodes { number headRefOid ` +
	`projectItems(first: 20) { pageInfo { hasNextPage } nodes { id isArchived project { id } } } ` +
	`closingIssuesReferences(first: 10) { pageInfo { hasNextPage } nodes { ` +
	`projectItems(first: 20) { pageInfo { hasNextPage } nodes { id isArchived project { id } } } } } } } } }`

type pageInfo struct {
	HasNextPage bool `json:"hasNextPage"`
}

type itemConn struct {
	PageInfo pageInfo `json:"pageInfo"`
	Nodes    []struct {
		ID         string              `json:"id"`
		IsArchived bool                `json:"isArchived"`
		Project    struct{ ID string } `json:"project"`
	} `json:"nodes"`
}

type readyNode struct {
	Number       int      `json:"number"`
	HeadRefOid   string   `json:"headRefOid"`
	ProjectItems itemConn `json:"projectItems"`
	Closing      struct {
		PageInfo pageInfo `json:"pageInfo"`
		Nodes    []struct {
			ProjectItems itemConn `json:"projectItems"`
		} `json:"nodes"`
	} `json:"closingIssuesReferences"`
}

// complete reports whether every connection the node carries fit in one
// page: a card past a page boundary would be judged absent.
func (n readyNode) complete() bool {
	if n.ProjectItems.PageInfo.HasNextPage || n.Closing.PageInfo.HasNextPage {
		return false
	}
	for _, c := range n.Closing.Nodes {
		if c.ProjectItems.PageInfo.HasNextPage {
			return false
		}
	}
	return true
}

// readyPullRequests runs readyQuery. A listing that does not fit, or any
// GraphQL error, fails the read: the Merger never decides on a partial list.
func (c *Client) readyPullRequests(ctx context.Context, base string) ([]readyNode, error) {
	var out struct {
		Data struct {
			Repository *struct {
				PullRequests struct {
					PageInfo pageInfo    `json:"pageInfo"`
					Nodes    []readyNode `json:"nodes"`
				} `json:"pullRequests"`
			} `json:"repository"`
		} `json:"data"`
		Errors []graphQLError `json:"errors"`
	}
	code, err := c.graphQL(ctx, graphQLBody{Query: readyQuery, Variables: map[string]string{
		"owner": c.opts.Repo.Owner, "name": c.opts.Repo.Name, "base": base, "label": LabelReadyToMerge,
	}}, &out)
	switch {
	case err != nil:
		return nil, err
	case code != http.StatusOK:
		return nil, fmt.Errorf("%w: POST graphql: ready listing: status %d", ErrGitHub, code)
	case len(out.Errors) > 0 || out.Data.Repository == nil:
		return nil, fmt.Errorf("%w: POST graphql: ready listing: %d errors", ErrGitHub, len(out.Errors))
	case out.Data.Repository.PullRequests.PageInfo.HasNextPage:
		return nil, fmt.Errorf("%w: POST graphql: ready listing: more than one page", ErrGitHub)
	}
	nodes := out.Data.Repository.PullRequests.Nodes
	for _, n := range nodes {
		if !n.complete() || n.Number <= 0 || !validSHA(n.HeadRefOid) {
			return nil, fmt.Errorf("%w: POST graphql: ready listing: pull request %d incomplete", ErrGitHub, n.Number)
		}
	}
	return nodes, nil
}
