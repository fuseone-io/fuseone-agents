package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A signed GitHub delivery attributes its run to the issue or pull request it
is about (#204, slice 3): github:<owner>/<repo>#<n>, from the fields GitHub
writes, only once the signature has proved the delivery, and without
changing anything else the door does — the body stays untrusted.
*/

const issueOpened = `{"action":"opened","repository":{"full_name":"O/R"},
	"issue":{"number":12,"title":"github:evil/x#1"}}`

// startOf is the opening step of the only run in the store.
func startOf(t *testing.T, store *ledger.Memory) (domain.Step, domain.RunStartedPayload) {
	t.Helper()
	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Fatalf("runs = %v, want one", runs)
	}
	steps, _ := store.Read(context.Background(), runs[0], domain.FirstSeq)
	var p domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &p); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	return steps[0], p
}

func TestGitHubHook_aSignedIssuesDelivery_attributesTheRunToTheIssue(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)

	rec := ghPost(t, mux, ghDelivery{path: "gh/reviewer", event: "issues", delivery: "d-issue",
		signature: ghSign(ghSecret, issueOpened), body: issueOpened})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s), want 202", rec.Code, rec.Body)
	}
	start, payload := startOf(t, store)
	want := domain.RunWorkItem{Ref: "github:o/r#12", Source: domain.WorkItemFromGitHubDelivery}
	if payload.WorkItem == nil || *payload.WorkItem != want {
		t.Fatalf("work item = %+v, want %+v", payload.WorkItem, want)
	}
	if !start.Labels.Has(domain.LabelUntrusted) {
		t.Errorf("labels = %v; the body is still untrusted", start.Labels)
	}
}

// A delivery the trigger wants but whose body names no card still opens its
// run, attributed to nothing.
func TestGitHubHook_aDeliveryNamingNoCard_opensWithout(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)

	if rec := ghPost(t, mux, opened(prOpened)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if _, payload := startOf(t, store); payload.WorkItem != nil {
		t.Fatalf("work item = %+v, want none", payload.WorkItem)
	}
}

// An ordinary hook is authenticated by a secret, not signed by GitHub: a
// body shaped like a GitHub delivery, with GitHub's event header, attributes
// nothing — whoever holds the secret could write any repository into it.
func TestHook_anOrdinaryHookWithAGitHubShapedBody_attributesNothing(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)

	req := ghDelivery{path: "crm/ticket", event: "issues", body: issueOpened}
	rec := postWithHeaders(t, mux, req, map[string]string{"X-FuseOne-Secret": "s3cret", "Idempotency-Key": "k-1"})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s), want 202", rec.Code, rec.Body)
	}
	if _, payload := startOf(t, store); payload.WorkItem != nil {
		t.Fatalf("work item = %+v; an unsigned body named the card", payload.WorkItem)
	}
}

func TestGitHubHook_anUnsignedIssuesDelivery_opensNothing(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)

	rec := ghPost(t, mux, ghDelivery{path: "gh/reviewer", event: "issues", delivery: "d-x",
		signature: ghSign("not-the-secret", issueOpened), body: issueOpened})

	if rec.Code != http.StatusUnauthorized || runsIn(t, store) != 0 {
		t.Fatalf("status = %d, runs = %d; want 401 and nothing", rec.Code, runsIn(t, store))
	}
}

func postWithHeaders(
	t *testing.T, mux *http.ServeMux, d ghDelivery, headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/hooks/"+d.path, strings.NewReader(d.body))
	req.Header.Set("X-GitHub-Event", d.event)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}
