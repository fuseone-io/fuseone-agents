package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

// Conflict detection as a client meets it, through the router serve mounts and against
// real Postgres. Headers are the contract here, so each request is written out.
type etagCall struct {
	method, path string
	ifMatch      []string
	body         any
}

func (w *readBackWorld) call(t *testing.T, c etagCall) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(c.body)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequestWithContext(context.WithoutCancel(t.Context()), c.method, "/api/v1"+c.path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for _, line := range c.ifMatch {
		req.Header.Add("If-Match", line)
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin}},
	}))
	rec := httptest.NewRecorder()
	w.route.ServeHTTP(rec, req)
	return rec
}

func aPolicyBody(name string) map[string]any {
	return map[string]any{"name": name, "effect": "deny", "mode": "enforce"}
}

// aStoredPolicy writes a rule, and removes it afterwards: the journeys share a database,
// and a deny left in force would change what the others decide.
func aStoredPolicy(t *testing.T, w *readBackWorld, code string) (path, etag string) {
	t.Helper()
	path = "/policies/" + code
	t.Cleanup(func() { w.call(t, etagCall{method: http.MethodDelete, path: path}) })
	put := w.call(t, etagCall{method: http.MethodPut, path: path, body: aPolicyBody("first")})
	if put.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", put.Code, put.Body)
	}
	return path, put.Header().Get("ETag")
}

func TestPolicyETag_putThenGetThenPutUnchanged_holdsStill(t *testing.T) {
	w := newReadBackWorld(t)
	path, written := aStoredPolicy(t, w, "ETAG-STILL")
	if !strings.HasPrefix(written, `"v1-`) || !strings.HasSuffix(written, `"`) {
		t.Fatalf("ETag = %s, want a strong quoted tag", written)
	}
	got := w.call(t, etagCall{method: http.MethodGet, path: path})
	if got.Code != http.StatusOK || got.Header().Get("ETag") != written {
		t.Fatalf("GET = %d with ETag %s, PUT said %s", got.Code, got.Header().Get("ETag"), written)
	}

	var before string
	state := `select updated_at::text || (select count(*) from policy_snapshots)::text from policies where code = 'ETAG-STILL'`
	if err := testdb.Pool(t).QueryRow(t.Context(), state).Scan(&before); err != nil {
		t.Fatalf("state: %v", err)
	}
	again := w.call(t, etagCall{method: http.MethodPut, path: path, ifMatch: []string{written}, body: aPolicyBody("first")})
	if again.Code != http.StatusOK || again.Header().Get("ETag") != written {
		t.Fatalf("PUT again = %d with ETag %s", again.Code, again.Header().Get("ETag"))
	}
	var after string
	if err := testdb.Pool(t).QueryRow(t.Context(), state).Scan(&after); err != nil {
		t.Fatalf("state: %v", err)
	}
	if after != before {
		t.Fatalf("an unchanged PUT moved the row: %s, then %s", before, after)
	}
}

func TestPolicyETag_aStaleWrite_isRefusedWithWhatIsThereNow(t *testing.T) {
	w := newReadBackWorld(t)
	path, first := aStoredPolicy(t, w, "ETAG-STALE")
	second := w.call(t, etagCall{method: http.MethodPut, path: path, ifMatch: []string{first}, body: aPolicyBody("second")})
	if second.Code != http.StatusOK || second.Header().Get("ETag") == first {
		t.Fatalf("a current write = %d with ETag %s", second.Code, second.Header().Get("ETag"))
	}

	stale := w.call(t, etagCall{method: http.MethodPut, path: path, ifMatch: []string{first}, body: aPolicyBody("third")})

	if stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale write = %d: %s", stale.Code, stale.Body)
	}
	if stale.Header().Get("ETag") != second.Header().Get("ETag") {
		t.Errorf("412 says %s, the resource is %s", stale.Header().Get("ETag"), second.Header().Get("ETag"))
	}
	if !strings.Contains(stale.Body.String(), `"fuseone:precondition-failed"`) {
		t.Errorf("body = %s", stale.Body)
	}
	if read := w.call(t, etagCall{method: http.MethodGet, path: path}); !strings.Contains(read.Body.String(), `"second"`) {
		t.Errorf("the refused write reached the row: %s", read.Body)
	}
}

func TestPolicyETag_headerForms_meanWhatHTTPSays(t *testing.T) {
	w := newReadBackWorld(t)
	path, current := aStoredPolicy(t, w, "ETAG-FORMS")
	cases := []struct {
		name  string
		lines []string
		want  int
	}{
		{"star", []string{`*`}, http.StatusOK},
		{"a list holding it", []string{`"v1-old", ` + current}, http.StatusOK},
		{"two lines holding it", []string{`"v1-old"`, current}, http.StatusOK},
		{"a list without it", []string{`"v1-old", "v1-older"`}, http.StatusPreconditionFailed},
		{"weak", []string{`W/` + current}, http.StatusPreconditionFailed},
		{"unquoted", []string{strings.Trim(current, `"`)}, http.StatusBadRequest},
		{"empty", []string{``}, http.StatusBadRequest},
		{"star in a list", []string{`*, ` + current}, http.StatusBadRequest},
	}
	for _, c := range cases {
		// The same body throughout: an accepted write stays a no-op, so the tag holds.
		got := w.call(t, etagCall{method: http.MethodPut, path: path, ifMatch: c.lines, body: aPolicyBody("first")})
		if got.Code != c.want {
			t.Errorf("%s = %d, want %d: %s", c.name, got.Code, c.want, got.Body)
		}
	}
}

func TestPolicyETag_onNothing_refusesAndDoesNotCreate(t *testing.T) {
	w := newReadBackWorld(t)
	path := "/policies/ETAG-NOBODY"
	t.Cleanup(func() { w.call(t, etagCall{method: http.MethodDelete, path: path}) })
	for _, line := range []string{`*`, `"v1-anything"`} {
		got := w.call(t, etagCall{method: http.MethodPut, path: path, ifMatch: []string{line}, body: aPolicyBody("x")})
		if got.Code != http.StatusPreconditionFailed || got.Header().Get("ETag") != "" {
			t.Errorf("If-Match %s = %d with ETag %q", line, got.Code, got.Header().Get("ETag"))
		}
	}
	if got := w.call(t, etagCall{method: http.MethodGet, path: path}); got.Code != http.StatusNotFound {
		t.Fatalf("GET = %d: it was created", got.Code)
	}
}

func TestPolicyETag_delete_needsTheCurrentRevision(t *testing.T) {
	w := newReadBackWorld(t)
	path, current := aStoredPolicy(t, w, "ETAG-DELETE")
	cases := []struct {
		name, line string
		want       int
	}{
		{"stale", `"v1-old"`, http.StatusPreconditionFailed},
		{"malformed", `v1-old`, http.StatusBadRequest},
	}
	for _, c := range cases {
		if got := w.call(t, etagCall{method: http.MethodDelete, path: path, ifMatch: []string{c.line}}); got.Code != c.want {
			t.Errorf("%s = %d, want %d", c.name, got.Code, c.want)
		}
	}
	if got := w.call(t, etagCall{method: http.MethodGet, path: path}); got.Code != http.StatusOK {
		t.Fatalf("a refused delete removed it: %d", got.Code)
	}
	if got := w.call(t, etagCall{method: http.MethodDelete, path: path, ifMatch: []string{current}}); got.Code != http.StatusNoContent {
		t.Fatalf("a current delete = %d", got.Code)
	}
	if got := w.call(t, etagCall{method: http.MethodDelete, path: path, ifMatch: []string{current}}); got.Code != http.StatusPreconditionFailed {
		t.Fatalf("deleting it twice = %d", got.Code)
	}
}

// Without the header nothing about today's callers changes: the last write still wins.
func TestPolicyETag_noHeader_lastWriteWins(t *testing.T) {
	w := newReadBackWorld(t)
	path, _ := aStoredPolicy(t, w, "ETAG-LWW")
	if got := w.call(t, etagCall{method: http.MethodPut, path: path, body: aPolicyBody("second")}); got.Code != http.StatusOK {
		t.Fatalf("PUT = %d", got.Code)
	}
	if read := w.call(t, etagCall{method: http.MethodGet, path: path}); !strings.Contains(read.Body.String(), `"second"`) {
		t.Fatalf("GET = %s", read.Body)
	}
}

// Writers started together do not overlap, because opening a connection takes longer than a
// write. So the row is held until every one of them has read and is waiting on a lock.
func TestPolicyETag_twoWritersWithOneETag_exactlyOneChangesIt(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	w := readBackWorldOn(t, pool)
	path, first := aStoredPolicy(t, w, "ETAG-RACE")
	hold, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = hold.Rollback(ctx) }()
	if _, err := hold.Exec(ctx, `select 1 from policies where code = 'ETAG-RACE' for update`); err != nil {
		t.Fatalf("hold: %v", err)
	}

	const writers = 4
	codes := make([]int, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			codes[i] = w.call(t, etagCall{
				method: http.MethodPut, path: path, ifMatch: []string{first},
				body: aPolicyBody(fmt.Sprintf("writer %d", i)),
			}).Code
		})
	}
	deadline := time.Now().Add(20 * time.Second)
	for waiting := 0; waiting < writers; {
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d writers reached a lock", waiting, writers)
		}
		if err := pool.QueryRow(ctx, `select count(*) from pg_stat_activity
			where datname = current_database() and wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatalf("watch: %v", err)
		}
	}
	if err := hold.Rollback(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}
	wg.Wait()

	won, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			won++
		case http.StatusPreconditionFailed:
			refused++
		default:
			t.Fatalf("a writer got %d", code)
		}
	}
	if won != 1 || refused != writers-1 {
		t.Fatalf("codes = %v", codes)
	}
}
