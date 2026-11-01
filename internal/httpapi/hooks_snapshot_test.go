package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

// echoRegistry publishes whichever agent it is asked about, so the run shows
// which agent the door chose.
type echoRegistry struct{}

func (echoRegistry) Versions(_ context.Context, agent domain.AgentID) ([]domain.AgentSummary, error) {
	return []domain.AgentSummary{{
		ID: agent, VersionID: "v1", Scope: domain.Scope{Company: "acme", Area: "cx"},
	}}, nil
}

// changingOwner is the store while a path changes hands: the row whose key a
// delivery proves belongs to triage, and any later read of the path finds it
// already billing's.
type changingOwner struct{ *fakeHooks }

func (c changingOwner) Find(ctx context.Context, path string) (trigger.Hook, error) {
	hook, err := c.fakeHooks.Find(ctx, path)
	hook.Agent = "billing"
	return hook, err
}

/*
The run is opened for the hook whose key the delivery proved.

Checking the key on one read and choosing the agent on a second one let a
delivery authenticated against triage's row open a run for whichever agent
held the path a moment later. One read returns the hook and is the one checked.
*/
func TestHook_theRunIsOpenedForTheHookWhoseKeyWasChecked(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*http.ServeMux) int{
		"an ordinary hook": func(mux *http.ServeMux) int {
			return post(t, mux, "crm/ticket", "s3cret", "d-1", `{"x":1}`).Code
		},
		"a GitHub hook": func(mux *http.ServeMux) int {
			return ghPost(t, mux, opened(prOpened)).Code
		},
	}
	for name, deliver := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			hooks := changingOwner{&fakeHooks{
				armed:   map[string]string{"crm/ticket": "s3cret"},
				agents:  map[string]domain.AgentID{"crm/ticket": "triage", "gh/reviewer": "triage"},
				signing: map[string]string{"gh/reviewer": ghSecret},
				events:  map[string][]string{"gh/reviewer": {"pull_request.opened"}},
			}}
			mux := http.NewServeMux()
			NewHooks(hooks, trigger.NewOpener(store, echoRegistry{}, hookClock{}),
				slog.New(slog.NewTextHandler(io.Discard, nil))).Mount(mux)

			if code := deliver(mux); code != http.StatusAccepted {
				t.Fatalf("status = %d, want the run opened", code)
			}
			runs, _ := store.Runs(context.Background())
			steps, _ := store.Read(context.Background(), runs[0], domain.FirstSeq)
			if steps[0].AgentID != "triage" {
				t.Errorf("run opened for %q, want triage: the hook whose key was checked", steps[0].AgentID)
			}
		})
	}
}
