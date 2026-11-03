package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
)

const (
	fakeToken = "fake-token"
	fakeOwner = "acme"
	fakeName  = "widgets"
)

// fakeThing is an issue or a pull request as the fake holds it.
type fakeThing struct {
	isPR      bool
	onBoard   bool
	updatedAt time.Time
	author    map[string]any
	merged    bool
	linked    []int
	nodes     []map[string]any // timeline items, in timeline order
}

type fakeRequest struct{ Method, Path, Body string }

// fakeGitHub is GitHub's GraphQL and REST APIs as far as factory-measure
// reads them. Like GitHub it refuses a page larger than 100, answers only
// the timeline item types a query asks for, pages by cursor, and pages REST
// lists through a Link header. It accepts only its own bearer.
type fakeGitHub struct {
	t *testing.T

	mu         sync.Mutex
	srv        *httptest.Server
	things     map[int]*fakeThing
	statuses   map[string][]map[string]any // sha -> statuses, newest first
	requests   []fakeRequest
	restPage   int    // REST page size; GitHub's per_page, capped by the fake
	failTypes  string // answers every timeline query with this GraphQL error type
	noProject  bool   // the token cannot see the project
	nullActors bool   // status events come back with project: null
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{t: t, things: map[int]*fakeThing{}, statuses: map[string][]map[string]any{}, restPage: 100}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) env() map[string]string {
	return map[string]string{
		"GITHUB_TOKEN":       fakeToken,
		"GITHUB_GRAPHQL_URL": f.srv.URL + "/graphql",
		"GITHUB_API_URL":     f.srv.URL,
	}
}

func (f *fakeGitHub) add(n int, th *fakeThing) *fakeThing {
	if th.author == nil {
		th.author = map[string]any{"__typename": "User", "login": "alice"}
	}
	f.things[n] = th
	return th
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if r.Method == http.MethodPost {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	f.requests = append(f.requests, fakeRequest{r.Method, r.URL.RequestURI(), body.Query})
	switch {
	case r.Header.Get("Authorization") != "Bearer "+fakeToken:
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Bad credentials"})
	case r.Method == http.MethodPost && r.URL.Path == "/graphql":
		f.graphql(w, body.Query, body.Variables)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/statuses"):
		f.restStatuses(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	}
}

func (f *fakeGitHub) graphql(w http.ResponseWriter, q string, vars map[string]any) {
	switch {
	case !strings.HasPrefix(strings.TrimSpace(q), "query"):
		writeJSON(w, http.StatusOK, fakeGQLError("FORBIDDEN", "the fake only answers queries"))
	case strings.Contains(q, "projectV2("):
		f.board(w, vars)
	case strings.Contains(q, "pullRequests("):
		f.pulls(w, vars)
	default:
		f.timelines(w, q, vars)
	}
}

func fakeGQLError(kind, msg string) map[string]any {
	return map[string]any{"data": nil, "errors": []map[string]any{{"type": kind, "message": msg}}}
}

func (f *fakeGitHub) numbers(keep func(*fakeThing) bool) []int {
	var ns []int
	for n, th := range f.things {
		if keep(th) {
			ns = append(ns, n)
		}
	}
	sort.Ints(ns)
	return ns
}

// page cuts list at the cursor ("" or an index) and returns pageInfo.
func page[T any](list []T, after any, first int) ([]T, map[string]any) {
	start := 0
	if s, ok := after.(string); ok && s != "" {
		start, _ = strconv.Atoi(s)
	}
	end := min(start+first, len(list))
	return list[start:end], map[string]any{"hasNextPage": end < len(list), "endCursor": strconv.Itoa(end)}
}

func (f *fakeGitHub) board(w http.ResponseWriter, vars map[string]any) {
	if f.noProject || vars["number"] != float64(project) {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"organization": map[string]any{"projectV2": nil}}})
		return
	}
	var nodes []map[string]any
	for _, n := range f.numbers(func(th *fakeThing) bool { return th.onBoard }) {
		kind := "Issue"
		if f.things[n].isPR {
			kind = "PullRequest"
		}
		nodes = append(nodes, map[string]any{"content": map[string]any{"__typename": kind, "number": n,
			"repository": map[string]any{"nameWithOwner": fakeOwner + "/" + fakeName}}})
	}
	nodes = append(nodes, map[string]any{"content": map[string]any{"__typename": "DraftIssue"}})
	got, info := page(nodes, vars["after"], 100)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"organization": map[string]any{
		"projectV2": map[string]any{"items": map[string]any{"pageInfo": info, "nodes": got}}}}})
}

func (f *fakeGitHub) pulls(w http.ResponseWriter, vars map[string]any) {
	ns := f.numbers(func(th *fakeThing) bool { return th.isPR })
	sort.SliceStable(ns, func(i, j int) bool { return f.things[ns[i]].updatedAt.After(f.things[ns[j]].updatedAt) })
	var nodes []map[string]any
	for _, n := range ns {
		nodes = append(nodes, map[string]any{"number": n, "updatedAt": f.things[n].updatedAt.Format(time.RFC3339)})
	}
	got, info := page(nodes, vars["after"], 100)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"repository": map[string]any{
		"pullRequests": map[string]any{"pageInfo": info, "nodes": got}}}})
}

var aliasRE = regexp.MustCompile(`(t\d+): (issue|pullRequest)\(number: (\d+)\) \{ timelineItems\(first: (\d+), after: \$(c\d+), itemTypes: \[([A-Z0-9_, ]+)\]\)`)

func (f *fakeGitHub) timelines(w http.ResponseWriter, q string, vars map[string]any) {
	if f.failTypes != "" {
		writeJSON(w, http.StatusOK, fakeGQLError(f.failTypes, "Resource not accessible by integration"))
		return
	}
	repo := map[string]any{}
	for _, m := range aliasRE.FindAllStringSubmatch(q, -1) {
		n, _ := strconv.Atoi(m[3])
		first, _ := strconv.Atoi(m[4])
		th := f.things[n]
		if first > 100 {
			writeJSON(w, http.StatusOK, fakeGQLError("MAX_PAGE_SIZE_EXCEEDED", "first must be at most 100"))
			return
		}
		if th == nil || th.isPR != (m[2] == "pullRequest") {
			repo[m[1]] = nil
			continue
		}
		repo[m[1]] = f.timelinePage(n, th, vars[m[5]], first, strings.Split(m[6], ", "))
	}
	if len(repo) == 0 {
		f.t.Errorf("the fake understood no alias in %q", q)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"repository": repo}})
}

func (f *fakeGitHub) timelinePage(n int, th *fakeThing, after any, first int, types []string) map[string]any {
	var nodes []map[string]any
	for _, node := range th.nodes {
		if slices.Contains(types, itemType(node["__typename"].(string))) {
			nodes = append(nodes, f.shaped(node))
		}
	}
	got, info := page(nodes, after, first)
	refs := map[string]any{"nodes": f.refs(th.linked)}
	out := map[string]any{"timelineItems": map[string]any{"pageInfo": info, "nodes": got}}
	if th.isPR {
		out["merged"], out["author"], out["closingIssuesReferences"] = th.merged, th.author, refs
	} else {
		out["closedByPullRequestsReferences"] = refs
	}
	return out
}

func (f *fakeGitHub) shaped(node map[string]any) map[string]any {
	if f.nullActors && node["__typename"] == "ProjectV2ItemStatusChangedEvent" {
		c := map[string]any{}
		for k, v := range node {
			c[k] = v
		}
		c["project"] = nil
		return c
	}
	return node
}

func (f *fakeGitHub) refs(ns []int) []map[string]any {
	var out []map[string]any
	for _, n := range ns {
		out = append(out, map[string]any{"number": n, "repository": map[string]any{"nameWithOwner": fakeOwner + "/" + fakeName}})
	}
	return append(out, map[string]any{"number": 9999, "repository": map[string]any{"nameWithOwner": "other/repo"}})
}

// itemType is a timeline __typename as its itemTypes enum value:
// ProjectV2ItemStatusChangedEvent -> PROJECT_V2_ITEM_STATUS_CHANGED_EVENT.
func itemType(typename string) string {
	var b strings.Builder
	rs := []rune(typename)
	for i, r := range rs {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(rs[i-1]) {
			b.WriteRune('_')
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

var statusPathRE = regexp.MustCompile(`^/repos/` + fakeOwner + `/` + fakeName + `/commits/([0-9a-f]{40})/statuses$`)

func (f *fakeGitHub) restStatuses(w http.ResponseWriter, r *http.Request) {
	m := statusPathRE.FindStringSubmatch(r.URL.Path)
	if m == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	per = min(max(per, 1), f.restPage)
	pg, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pg = max(pg, 1)
	list := f.statuses[m[1]]
	start, end := min((pg-1)*per, len(list)), min(pg*per, len(list))
	if end < len(list) {
		w.Header().Set("Link", fmt.Sprintf(`<%s%s?per_page=%d&page=%d>; rel="next"`, f.srv.URL, r.URL.Path, per, pg+1))
	}
	writeJSON(w, http.StatusOK, list[start:end])
}

// Timeline node builders, shaped as GitHub answers them.

func ghActor(a Actor) any {
	if a.Type == "" {
		return nil
	}
	return map[string]any{"__typename": a.Type, "login": a.Login}
}

func nStatus(when, from, to string, who Actor) map[string]any {
	return map[string]any{"__typename": "ProjectV2ItemStatusChangedEvent", "createdAt": when,
		"previousStatus": from, "status": to, "project": map[string]any{"number": project}, "actor": ghActor(who)}
}

func nComment(when, body string, who Actor) map[string]any {
	return map[string]any{"__typename": "IssueComment", "createdAt": when, "author": ghActor(who), "body": body}
}

func nLabel(when string, on bool, name string, who Actor) map[string]any {
	kind := "UnlabeledEvent"
	if on {
		kind = "LabeledEvent"
	}
	return map[string]any{"__typename": kind, "createdAt": when, "actor": ghActor(who), "label": map[string]any{"name": name}}
}

func nReview(when, state, sha string, who Actor) map[string]any {
	return map[string]any{"__typename": "PullRequestReview", "submittedAt": when, "state": state,
		"author": ghActor(who), "commit": map[string]any{"oid": sha}}
}

func nReviewID(when, state, sha, id string, who Actor) map[string]any {
	n := nReview(when, state, sha, who)
	n["id"] = id
	return n
}

func nDismissed(when, reviewID, previous string, who Actor) map[string]any {
	return map[string]any{"__typename": "ReviewDismissedEvent", "createdAt": when, "actor": ghActor(who),
		"previousReviewState": previous, "review": map[string]any{"id": reviewID}}
}

// nCommit is a commit by a person (a linked user) or by an App, whose
// author GitHub gives no user but a noreply address.
func nCommit(when, sha string, who Actor) map[string]any {
	author := map[string]any{"email": "someone@example.com", "user": nil}
	switch who.Type {
	case "User":
		author["user"] = map[string]any{"login": who.Login}
	case "Bot":
		author["email"] = "41898282+" + who.Login + "@users.noreply.github.com"
	}
	return map[string]any{"__typename": "PullRequestCommit",
		"commit": map[string]any{"oid": sha, "committedDate": when, "author": author}}
}

func nEvent(kind, when string, who Actor) map[string]any {
	return map[string]any{"__typename": kind, "createdAt": when, "actor": ghActor(who)}
}

func rStatus(when, state, context string, who Actor) map[string]any {
	return map[string]any{"state": state, "context": context, "created_at": when,
		"creator": map[string]any{"login": who.Login, "type": who.Type}, "description": "free text"}
}
