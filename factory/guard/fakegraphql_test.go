package guard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeItem is a card as the fake holds it. Field values are option ids.
type fakeItem struct {
	ID, ProjectID    string
	Archived         bool
	Status, WorkType string
	Size             string
}

// fakeMutation is one updateProjectV2ItemFieldValue as the fake received it.
type fakeMutation struct {
	Token                        string
	Project, Item, Field, Option string
}

// fakeGraphQL is GitHub's GraphQL API, as far as the board tools use it.
// Like GitHub, it refuses an update to an item outside the project, to a
// field the project lacks, or with an option the field does not have. It
// plants the canary in every title, every text value and every error
// message.
type fakeGraphQL struct {
	t *testing.T

	mu             sync.Mutex
	board          BoardConfig
	items          map[string]*fakeItem
	bots           map[string]int64 // bearer -> the bot user it belongs to
	tokens         []string         // every bearer seen, in order
	mutations      []fakeMutation   // every update received, applied or not
	viewerCalls    int
	beforeMutation func(f *fakeGraphQL) // runs as an update arrives, after every read
	refuseField    string               // answers errors[] for updates to this field
	mutationStatus int                  // forces a status on updates
	dropMutation   bool                 // applies the update, then loses the answer
	readErrors     bool                 // answers every item read with a non-NOT_FOUND error
	prs            []*fakeGQLPR         // pull requests, in creation order
	morePRs        bool                 // the listing claims another page
}

// fakeGQLPR is a pull request as the ready listing sees it: its head, base,
// labels, its own project items and those of the issues it closes.
type fakeGQLPR struct {
	Number  int
	Head    string
	Base    string
	Open    bool
	Merged  bool
	Labels  []string
	Items   []string
	Closing [][]string
}

func (f *fakeGraphQL) itemsJSON(ids []string) map[string]any {
	nodes := []map[string]any{}
	for _, id := range ids {
		it := f.items[id]
		nodes = append(nodes, map[string]any{"id": id, "isArchived": it.Archived, "type": canary,
			"project": map[string]any{"id": it.ProjectID, "title": canary}})
	}
	return map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": nodes}
}

// listReady answers the open and the merged pull requests on the base that
// carry the label, oldest first, as GitHub's connection filtered by states
// OPEN and MERGED does: one closed without a merge is in neither.
//
// The states are read from the query itself, as GitHub reads them: a query
// that asks for OPEN only is answered with no merged pull request.
func (f *fakeGraphQL) listReady(w http.ResponseWriter, q string, v map[string]string) {
	open, merged := statesAsked(q)
	nodes := []map[string]any{}
	for _, pr := range f.prs {
		if !(open && pr.Open || merged && pr.Merged) || pr.Base != v["base"] || !contains(pr.Labels, v["label"]) {
			continue
		}
		state := "OPEN"
		if pr.Merged {
			state = "MERGED"
		}
		closing := []map[string]any{}
		for _, ids := range pr.Closing {
			closing = append(closing, map[string]any{"title": canary, "projectItems": f.itemsJSON(ids)})
		}
		nodes = append(nodes, map[string]any{"number": pr.Number, "headRefOid": pr.Head, "state": state, "title": canary,
			"body": canary, "projectItems": f.itemsJSON(pr.Items),
			"closingIssuesReferences": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": closing}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"repository": map[string]any{
		"pullRequests": map[string]any{"pageInfo": map[string]any{"hasNextPage": f.morePRs}, "nodes": nodes},
	}}})
}

func newFakeGraphQL(t *testing.T, board BoardConfig) (*fakeGraphQL, *httptest.Server) {
	t.Helper()
	f := &fakeGraphQL{t: t, board: board, items: map[string]*fakeItem{}, bots: map[string]int64{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeGraphQL) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	f.tokens = append(f.tokens, token)
	if _, ok := f.bots[token]; !ok || r.Method != http.MethodPost || r.URL.Path != "/graphql" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": canary})
		return
	}
	var body graphQLBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": canary})
		return
	}
	switch q := strings.TrimSpace(body.Query); {
	case strings.HasPrefix(q, "mutation"):
		f.update(w, token, body.Variables)
	case strings.Contains(q, "pullRequests("):
		f.listReady(w, q, body.Variables)
	case strings.Contains(q, "viewer"):
		f.viewerCalls++
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"viewer": map[string]any{"databaseId": f.bots[token], "login": canary, "name": canary},
		}})
	default:
		f.readItem(w, body.Variables["id"])
	}
}

func gqlErrors(data any, kind string) map[string]any {
	return map[string]any{"data": data, "errors": []map[string]any{{"type": kind, "message": canary}}}
}

func (f *fakeGraphQL) readItem(w http.ResponseWriter, id string) {
	item := f.items[id]
	switch {
	case f.readErrors:
		writeJSON(w, http.StatusOK, gqlErrors(map[string]any{"node": nil}, "FORBIDDEN"))
		return
	case item == nil:
		writeJSON(w, http.StatusOK, gqlErrors(map[string]any{"node": nil}, "NOT_FOUND"))
		return
	}
	values := []map[string]any{
		{"text": canary, "field": map[string]any{"id": "PVTF_title", "name": canary}},
	}
	for field, option := range map[string]string{
		f.board.StatusFieldID: item.Status, f.board.WorkTypeFieldID: item.WorkType, f.board.SizeFieldID: item.Size,
	} {
		if option != "" {
			values = append(values, map[string]any{"optionId": option, "name": canary,
				"field": map[string]any{"id": field, "name": canary}})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"node": map[string]any{
		"id": item.ID, "isArchived": item.Archived, "title": canary,
		"content":     map[string]any{"title": canary, "body": canary},
		"project":     map[string]any{"id": item.ProjectID, "title": canary},
		"fieldValues": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": values},
	}}})
}

// slot returns where the item keeps the field's value, and the options the
// field has; nil for a field the project does not have.
func (f *fakeGraphQL) slot(item *fakeItem, field string) (*string, []string) {
	switch field {
	case f.board.StatusFieldID:
		return &item.Status, mapValues(f.board.Lanes)
	case f.board.WorkTypeFieldID:
		return &item.WorkType, mapValues(f.board.WorkTypes)
	case f.board.SizeFieldID:
		return &item.Size, mapValues(f.board.Sizes)
	}
	return nil, nil
}

func mapValues[K comparable](m map[K]string) []string {
	var out []string
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func (f *fakeGraphQL) update(w http.ResponseWriter, token string, v map[string]string) {
	f.mutations = append(f.mutations, fakeMutation{Token: token, Project: v["project"], Item: v["item"],
		Field: v["field"], Option: v["option"]})
	if f.beforeMutation != nil {
		f.beforeMutation(f)
	}
	if f.mutationStatus != 0 {
		writeJSON(w, f.mutationStatus, map[string]string{"message": canary})
		return
	}
	item := f.items[v["item"]]
	if item == nil || item.ProjectID != v["project"] || v["project"] != f.board.ProjectID || f.refuseField == v["field"] {
		writeJSON(w, http.StatusOK, gqlErrors(map[string]any{"updateProjectV2ItemFieldValue": nil}, "NOT_FOUND"))
		return
	}
	value, opts := f.slot(item, v["field"])
	if value == nil || !contains(opts, v["option"]) {
		writeJSON(w, http.StatusOK, gqlErrors(map[string]any{"updateProjectV2ItemFieldValue": nil}, "UNPROCESSABLE"))
		return
	}
	*value = v["option"]
	if f.dropMutation {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			_ = conn.Close()
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updateProjectV2ItemFieldValue": map[string]any{
		"projectV2Item": map[string]any{"id": item.ID, "title": canary},
	}}})
}

// statesAsked reads the states argument of the pullRequests connection.
func statesAsked(q string) (open, merged bool) {
	_, args, _ := strings.Cut(q, "pullRequests(")
	args, _, _ = strings.Cut(args, ")")
	_, states, _ := strings.Cut(args, "states:")
	states, _, _ = strings.Cut(states, ", baseRefName")
	return strings.Contains(states, "OPEN"), strings.Contains(states, "MERGED")
}
