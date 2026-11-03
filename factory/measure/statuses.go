package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var shaRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

type rawStatus struct {
	State     string    `json:"state"`
	Context   string    `json:"context"`
	CreatedAt time.Time `json:"created_at"`
	Creator   *struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"creator"`
}

// qaStatuses reads, through REST, the factory/qa statuses on every commit a
// factory pull request carried or was reviewed at. A pull request no role
// touched is skipped: none of its statuses could be QA's.
func (r reader) qaStatuses(ctx context.Context, s Snapshot) error {
	for _, n := range sortedNumbers(s) {
		pr := s.Items[n]
		if !pr.IsPR || !r.factoryPR(pr) {
			continue
		}
		for _, sha := range shasOf(pr) {
			if err := r.statusesOf(ctx, pr, sha); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r reader) statusesOf(ctx context.Context, pr *Item, sha string) error {
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/statuses?per_page=100", r.owner, r.name, sha)
	return r.c.getPages(ctx, path, func(page json.RawMessage) error {
		var list []rawStatus
		if err := json.Unmarshal(page, &list); err != nil {
			return fmt.Errorf("%w: statuses of %.7s: %w", errGitHub, sha, err)
		}
		for _, st := range list {
			if st.Context != qaContext {
				continue
			}
			var creator Actor
			if st.Creator != nil {
				creator = Actor{Type: st.Creator.Type, Login: st.Creator.Login}
			}
			pr.Statuses = append(pr.Statuses, Status{SHA: sha, Context: st.Context, State: st.State, At: st.CreatedAt, Creator: creator})
		}
		return nil
	})
}

// factoryPR says whether a role wrote it, acted on it, or labelled it.
func (r reader) factoryPR(pr *Item) bool {
	if r.bots.classify(pr.Author).isRole() {
		return true
	}
	for _, e := range pr.Events {
		if r.bots.classify(e.Actor).isRole() || strings.HasPrefix(e.Label, "factory:") {
			return true
		}
	}
	return false
}

// shasOf is every well-formed commit id the pull request's timeline names,
// in the order it first names them.
func shasOf(pr *Item) []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range pr.Events {
		if shaRE.MatchString(e.SHA) && !seen[e.SHA] {
			seen[e.SHA] = true
			out = append(out, e.SHA)
		}
	}
	return out
}
