package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

// GitHub's door: the same /hooks/{path} route, a path whose published trigger
// says `from: github`. GitHub signs instead of sending the secret, names the
// delivery in its own header, and sends every event the hook subscribes to.

const ghSecret = "gh-hook-secret"

type capturedInput struct {
	mu   sync.Mutex
	data [][]byte
}

func (c *capturedInput) Put(_ context.Context, _ domain.RunID, _ int64, data []byte) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = append(c.data, data)
	return "ref", nil
}

func ghServer(t *testing.T) (*http.ServeMux, *ledger.Memory, *capturedInput) {
	t.Helper()
	store := ledger.NewMemory()
	inputs := &capturedInput{}
	hooks := &fakeHooks{
		armed: map[string]string{"crm/ticket": "s3cret"},
		agents: map[string]domain.AgentID{
			"crm/ticket": "triage", "gh/reviewer": "triage", "gh/closed": "triage",
		},
		signing: map[string]string{"gh/reviewer": ghSecret, "gh/closed": ""},
		events:  map[string][]string{"gh/reviewer": {"pull_request.opened", "issues"}},
	}
	mux := http.NewServeMux()
	NewHooks(hooks, trigger.NewOpener(store, hookRegistry{}, hookClock{}).WithContent(inputs),
		slog.New(slog.NewTextHandler(io.Discard, nil))).Mount(mux)
	return mux, store, inputs
}

func ghSign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type ghDelivery struct {
	path, event, delivery, signature, body string
}

func ghPost(t *testing.T, mux *http.ServeMux, d ghDelivery) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/hooks/"+d.path, strings.NewReader(d.body))
	req.Header.Set("Content-Type", "application/json")
	if d.event != "" {
		req.Header.Set("X-GitHub-Event", d.event)
	}
	if d.delivery != "" {
		req.Header.Set("X-GitHub-Delivery", d.delivery)
	}
	if d.signature != "" {
		req.Header.Set("X-Hub-Signature-256", d.signature)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func opened(body string) ghDelivery {
	return ghDelivery{path: "gh/reviewer", event: "pull_request", delivery: "d-1",
		signature: ghSign(ghSecret, body), body: body}
}

const prOpened = `{"action":"opened","number":7}`

func runsIn(t *testing.T, store *ledger.Memory) int {
	t.Helper()
	runs, err := store.Runs(context.Background())
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	return len(runs)
}

func TestGitHubHook_signedDeliveryItWants_opensOneTaintedRun(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)

	rec := ghPost(t, mux, opened(prOpened))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (%s)", rec.Code, rec.Body)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Fatalf("runs = %v, want one", runs)
	}
	steps, _ := store.Read(context.Background(), runs[0], domain.FirstSeq)
	if !steps[0].Labels.Has(domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the delivery marked untrusted", steps[0].Labels)
	}
}

// The run's input says which delivery it was: GitHub's body does not carry
// its own event name, and an agent woken by two kinds of event needs it.
func TestGitHubHook_theRunsInput_namesTheEventAndCarriesThePayload(t *testing.T) {
	t.Parallel()
	mux, _, inputs := ghServer(t)

	if rec := ghPost(t, mux, opened(prOpened)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	var input struct {
		Event, Action, Delivery string
		Payload                 map[string]any
	}
	if len(inputs.data) != 1 || json.Unmarshal(inputs.data[0], &input) != nil {
		t.Fatalf("input = %q, want one JSON document", inputs.data)
	}
	if input.Event != "pull_request" || input.Action != "opened" || input.Delivery != "d-1" ||
		input.Payload["number"] != float64(7) {
		t.Errorf("input = %+v, want the event, action, delivery and the payload", input)
	}
}

/*
Everything that is not a valid signature answers the way a wrong secret does.

The same 401 and the same words for a missing signature, one under another
secret, a body changed after signing, a path that is not configured, and the
GitHub hook's secret sent as if it were an ordinary hook's: a caller probing
must not learn which paths exist or which come from GitHub.
*/
func TestGitHubHook_anythingButAValidSignature_isRejectedAlike(t *testing.T) {
	t.Parallel()

	cases := map[string]ghDelivery{
		"no signature":            {path: "gh/reviewer", event: "pull_request", delivery: "d", body: prOpened},
		"another secret":          {path: "gh/reviewer", event: "pull_request", delivery: "d", body: prOpened, signature: ghSign("x", prOpened)},
		"body changed":            {path: "gh/reviewer", event: "pull_request", delivery: "d", body: `{"action":"closed"}`, signature: ghSign(ghSecret, prOpened)},
		"a hook nobody rotated":   {path: "gh/closed", event: "pull_request", delivery: "d", body: prOpened, signature: ghSign("", prOpened)},
		"a path nobody declared":  {path: "gh/none", event: "pull_request", delivery: "d", body: prOpened, signature: ghSign(ghSecret, prOpened)},
		"signed for another hook": {path: "crm/ticket", event: "pull_request", delivery: "d", body: prOpened, signature: ghSign(ghSecret, prOpened)},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mux, store, _ := ghServer(t)
			rec := ghPost(t, mux, d)
			if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "The secret is missing or wrong.") {
				t.Errorf("status = %d (%s), want the uniform rejection", rec.Code, rec.Body)
			}
			if n := runsIn(t, store); n != 0 {
				t.Errorf("runs = %d, want none", n)
			}
		})
	}
}

func TestGitHubHook_secretSentAsAnOrdinaryHooks_isRejected(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)

	rec := post(t, mux, "gh/reviewer", ghSecret, "d-1", prOpened)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401: a GitHub hook's secret signs, it is never sent", rec.Code)
	}
	if n := runsIn(t, store); n != 0 {
		t.Errorf("runs = %d, want none", n)
	}
}

// GitHub redelivers with the same X-GitHub-Delivery, and the same delivery
// is always the same run.
func TestGitHubHook_redelivery_namesTheSameRun(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)

	first := ghPost(t, mux, opened(prOpened))
	second := ghPost(t, mux, opened(prOpened))

	if second.Code != http.StatusOK || runOf(t, first) != runOf(t, second) {
		t.Fatalf("redelivery = %d %s, want 200 naming the first run", second.Code, second.Body)
	}
	if n := runsIn(t, store); n != 1 {
		t.Errorf("runs = %d, want one", n)
	}
}

// Two deliveries of the same event are two runs: the delivery id is the key,
// not the event or the body.
func TestGitHubHook_twoDeliveries_openTwoRuns(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)
	second := opened(prOpened)
	second.delivery = "d-2"

	ghPost(t, mux, opened(prOpened))
	ghPost(t, mux, second)

	if n := runsIn(t, store); n != 2 {
		t.Errorf("runs = %d, want two", n)
	}
}

/*
A delivery the trigger did not ask for is acknowledged and starts nothing.

2xx, because GitHub retries anything else and marks the hook as failing; no
run, because the trigger names which events start one.
*/
func TestGitHubHook_deliveryItDidNotAskFor_isAcknowledgedWithNoRun(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ event, body string }{
		"another action of a filtered event": {"pull_request", `{"action":"closed"}`},
		"an event it did not name":           {"push", `{"ref":"refs/heads/main"}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mux, store, _ := ghServer(t)
			d := ghDelivery{path: "gh/reviewer", event: c.event, delivery: "d-x",
				signature: ghSign(ghSecret, c.body), body: c.body}

			rec := ghPost(t, mux, d)
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			if n := runsIn(t, store); n != 0 {
				t.Errorf("runs = %d, want none", n)
			}
		})
	}
}

// An event named without an action takes every action of it.
func TestGitHubHook_eventNamedWithoutAction_takesAnyAction(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)
	body := `{"action":"edited"}`

	rec := ghPost(t, mux, ghDelivery{path: "gh/reviewer", event: "issues", delivery: "d-i",
		signature: ghSign(ghSecret, body), body: body})

	if rec.Code != http.StatusAccepted || runsIn(t, store) != 1 {
		t.Errorf("status = %d, runs = %d; want the run opened", rec.Code, runsIn(t, store))
	}
}

// GitHub pings a hook when it is created. Signed, it is answered; it starts
// nothing.
func TestGitHubHook_ping_isAnsweredWithNoRun(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)
	body := `{"zen":"Keep it logically awesome.","hook_id":1}`

	rec := ghPost(t, mux, ghDelivery{path: "gh/reviewer", event: "ping", delivery: "d-p",
		signature: ghSign(ghSecret, body), body: body})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if n := runsIn(t, store); n != 0 {
		t.Errorf("runs = %d, want none", n)
	}
}

// A body over the limit is never read whole, so it cannot be verified: it is
// refused like any delivery nobody can vouch for, and opens nothing.
func TestGitHubHook_bodyOverTheLimit_isRefusedWithNoRun(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)
	body := `{"action":"opened","pad":"` + strings.Repeat("x", maxHookBody) + `"}`

	rec := ghPost(t, mux, opened(body))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want the uniform rejection", rec.Code)
	}
	if n := runsIn(t, store); n != 0 {
		t.Errorf("runs = %d, want none", n)
	}
}

// Signed but unnamed: the integrator who left out the header reads why.
func TestGitHubHook_signedWithNoDeliveryID_isRefusedWithAReason(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)
	d := opened(prOpened)
	d.delivery = ""

	rec := ghPost(t, mux, d)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "X-GitHub-Delivery") {
		t.Errorf("status = %d (%s), want 400 naming the header", rec.Code, rec.Body)
	}
	if n := runsIn(t, store); n != 0 {
		t.Errorf("runs = %d, want none", n)
	}
}

// GitHub can send a hook's body form-encoded. The signature is over those
// bytes, but the run's input has to be the JSON GitHub would send otherwise.
func TestGitHubHook_aBodyThatIsNotJSON_isRefusedWithAReason(t *testing.T) {
	t.Parallel()
	mux, store, _ := ghServer(t)
	body := "payload=%7B%22action%22%3A%22opened%22%7D"

	rec := ghPost(t, mux, ghDelivery{path: "gh/reviewer", event: "pull_request", delivery: "d-f",
		signature: ghSign(ghSecret, body), body: body})

	if rec.Code != http.StatusUnsupportedMediaType || !strings.Contains(rec.Body.String(), "application/json") {
		t.Errorf("status = %d (%s), want 415 naming application/json", rec.Code, rec.Body)
	}
	if n := runsIn(t, store); n != 0 {
		t.Errorf("runs = %d, want none", n)
	}
}
