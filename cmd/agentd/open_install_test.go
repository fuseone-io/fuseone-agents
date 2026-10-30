package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
)

// The in-memory demo is an installation with nobody to sign in as. It seeded
// and served, but every route still demanded a principal, so the console could
// neither list the demo's runs nor open an answer (#77, review). This crosses
// the real router: the open mount lists the seeded run and opens its closing
// answer, while the same API without that mount still refuses — the open
// principal is the mechanism, not something the handlers stopped checking.
func TestOpenInstallation_theSeededDemo_isListedAndItsAnswerOpens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := ledger.NewMemory()
	content := demoContent(nil)
	if err := seedDemo(ctx, store, content); err != nil {
		t.Fatalf("seed demo: %v", err)
	}
	api := apiHandlerFor(httpapi.NewServer(store, "test").WithContent(content))

	root := http.NewServeMux()
	mountOpen(root, api)
	open := httptest.NewServer(root)
	t.Cleanup(open.Close)

	listing := get(t, open.URL+"/api/v1/runs")
	if listing.status != http.StatusOK || !strings.Contains(listing.body, "run-triage-8801") {
		t.Fatalf("GET /runs = %d %s, want the seeded run listed", listing.status, listing.body)
	}

	seq := finishedSeq(t, store, "run-triage-8801")
	answer := get(t, fmt.Sprintf("%s/api/v1/runs/run-triage-8801/steps/%d/content", open.URL, seq))
	if answer.status != http.StatusOK || !strings.Contains(answer.body, "classified") {
		t.Fatalf("the finished run's answer = %d %s, want it opened", answer.status, answer.body)
	}

	guarded := httptest.NewServer(api)
	t.Cleanup(guarded.Close)
	if got := get(t, guarded.URL+"/api/v1/runs"); got.status == http.StatusOK {
		t.Fatalf("the API without the open mount answered %d: the handlers stopped checking", got.status)
	}
}

type response struct {
	status int
	body   string
}

func get(t *testing.T, url string) response {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return response{status: res.StatusCode, body: string(raw)}
}

func finishedSeq(t *testing.T, store *ledger.Memory, run domain.RunID) int64 {
	t.Helper()
	steps, err := store.Read(context.Background(), run, domain.FirstSeq)
	if err != nil {
		t.Fatalf("read %s: %v", run, err)
	}
	for _, step := range steps {
		if step.Kind == domain.StepRunFinished {
			return step.Seq
		}
	}
	t.Fatalf("%s never finished", run)
	return 0
}
