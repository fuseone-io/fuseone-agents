package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// reader reads one repository and one organization project.
type reader struct {
	c           *client
	owner, name string
	project     int
	bots        Bots
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type rawRef struct {
	Number     int `json:"number"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

func (r reader) ours(ref rawRef) bool {
	return strings.EqualFold(ref.Repository.NameWithOwner, r.owner+"/"+r.name)
}

// snapshot reads every card on the board, every pull request updated since
// the window opened, every issue or pull request those link to, and the
// factory/qa statuses of the pull requests the factory touched.
func (r reader) snapshot(ctx context.Context, since time.Time) (Snapshot, error) {
	board, err := r.boardCards(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	prs, err := r.recentPRs(ctx, since)
	if err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{Items: map[int]*Item{}}
	if err := r.readLinked(ctx, s, append(board, prs...)); err != nil {
		return Snapshot{}, err
	}
	for _, t := range board {
		s.Items[t.number].OnBoard = true
	}
	return s, r.qaStatuses(ctx, s)
}

// readLinked reads targets, then whatever they link to, until nothing new.
func (r reader) readLinked(ctx context.Context, s Snapshot, targets []target) error {
	for len(targets) > 0 {
		var fresh []target
		for _, t := range targets {
			if s.Items[t.number] == nil && !hasTarget(fresh, t.number) {
				fresh = append(fresh, t)
			}
		}
		if err := r.timelines(ctx, s, fresh); err != nil {
			return err
		}
		targets = nil
		for _, t := range fresh {
			it := s.Items[t.number]
			for _, n := range it.Linked {
				targets = append(targets, target{number: n, isPR: !it.IsPR})
			}
		}
	}
	return nil
}

func hasTarget(ts []target, n int) bool {
	for _, t := range ts {
		if t.number == n {
			return true
		}
	}
	return false
}

const boardQuery = `query($owner: String!, $number: Int!, $after: String) { organization(login: $owner) { projectV2(number: $number) { items(first: 100, after: $after) { pageInfo { hasNextPage endCursor } nodes { content { __typename ... on Issue { number repository { nameWithOwner } } ... on PullRequest { number repository { nameWithOwner } } } } } } } }`

// boardCards lists the project's issues and pull requests in this
// repository; draft items and other repositories' cards are skipped.
func (r reader) boardCards(ctx context.Context) ([]target, error) {
	var out []target
	var after any
	for {
		var res struct {
			Organization struct {
				ProjectV2 *struct {
					Items struct {
						PageInfo pageInfo `json:"pageInfo"`
						Nodes    []struct {
							Content struct {
								Typename string `json:"__typename"`
								rawRef
							} `json:"content"`
						} `json:"nodes"`
					} `json:"items"`
				} `json:"projectV2"`
			} `json:"organization"`
		}
		vars := map[string]any{"owner": r.owner, "number": r.project, "after": after}
		if err := r.c.query(ctx, boardQuery, vars, &res); err != nil {
			return nil, fmt.Errorf("board: %w", err)
		}
		p := res.Organization.ProjectV2
		if p == nil {
			return nil, fmt.Errorf("%w: project %d of %s is not readable with this token", errGitHub, r.project, r.owner)
		}
		for _, n := range p.Items.Nodes {
			c := n.Content
			if (c.Typename == "Issue" || c.Typename == "PullRequest") && r.ours(c.rawRef) {
				out = append(out, target{number: c.Number, isPR: c.Typename == "PullRequest"})
			}
		}
		if !p.Items.PageInfo.HasNextPage {
			return out, nil
		}
		after = p.Items.PageInfo.EndCursor
	}
}

const pullsQuery = `query($owner: String!, $name: String!, $after: String) { repository(owner: $owner, name: $name) { pullRequests(first: 100, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) { pageInfo { hasNextPage endCursor } nodes { number updatedAt } } } }`

// recentPRs lists the pull requests updated at or after since, newest first.
func (r reader) recentPRs(ctx context.Context, since time.Time) ([]target, error) {
	var out []target
	var after any
	for {
		var res struct {
			Repository struct {
				PullRequests struct {
					PageInfo pageInfo `json:"pageInfo"`
					Nodes    []struct {
						Number    int       `json:"number"`
						UpdatedAt time.Time `json:"updatedAt"`
					} `json:"nodes"`
				} `json:"pullRequests"`
			} `json:"repository"`
		}
		vars := map[string]any{"owner": r.owner, "name": r.name, "after": after}
		if err := r.c.query(ctx, pullsQuery, vars, &res); err != nil {
			return nil, fmt.Errorf("pull requests: %w", err)
		}
		prs := res.Repository.PullRequests
		for _, n := range prs.Nodes {
			if n.UpdatedAt.Before(since) {
				return out, nil
			}
			out = append(out, target{number: n.Number, isPR: true})
		}
		if !prs.PageInfo.HasNextPage {
			return out, nil
		}
		after = prs.PageInfo.EndCursor
	}
}
