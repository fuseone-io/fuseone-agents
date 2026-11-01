package spec_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

const cronLine = `  - { type: cron, schedule: "*/15 * * * *" }`

func withTrigger(line string) string {
	return strings.Replace(valid, cronLine, line, 1)
}

/*
A webhook can say it comes from GitHub, and which deliveries it wants.

GitHub signs a delivery instead of sending a shared secret, names it in its own
header instead of Idempotency-Key, and sends every event the hook subscribes
to. The trigger carries what the door needs to tell the deliveries apart: where
they come from and which event, or event and action, starts a run.
*/
func TestParse_aGitHubWebhook_carriesItsSourceAndEvents(t *testing.T) {
	t.Parallel()

	s := parse(t, withTrigger(
		`  - { type: webhook, path: factory/reviewer, from: github, events: [pull_request.opened, issues] }`))

	got := spec.Webhooks(s)
	want := []domain.WebhookTrigger{{
		Path: "factory/reviewer", From: domain.WebhookFromGitHub,
		Events: []string{"pull_request.opened", "issues"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Webhooks = %+v, want %+v", got, want)
	}
}

// Rendered and parsed again, the declaration is the same one: the console
// writes the file, and a field the renderer dropped would be a GitHub hook
// that quietly became an ordinary one at the next save.
func TestRender_aGitHubWebhook_survivesTheRoundTrip(t *testing.T) {
	t.Parallel()

	s := parse(t, withTrigger(
		`  - { type: webhook, path: factory/reviewer, from: github, events: [pull_request.labeled] }`))
	out, err := spec.Render(s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	again := parse(t, string(out))

	if !reflect.DeepEqual(again.Triggers, s.Triggers) {
		t.Fatalf("triggers after the round trip = %+v, want %+v", again.Triggers, s.Triggers)
	}
}

// A webhook that declares no source renders exactly as before, so every
// version published before GitHub hooks existed keeps its digest.
func TestRender_anOrdinaryWebhook_writesNoSourceAndNoEvents(t *testing.T) {
	t.Parallel()

	out, err := spec.Render(parse(t, withTrigger(`  - { type: webhook, path: crm/ticket }`)))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(string(out), "from:") || strings.Contains(string(out), "events:") {
		t.Errorf("an ordinary webhook rendered a source or events:\n%s", out)
	}
}

/*
Published and read back, a GitHub webhook keeps its source and events on both
of the registry's readers: Get, which the worker and the door's sync read, and
Versions, which the console's form is filled from. Either one dropping them
turns the hook ordinary at the next sync or the next save.
*/
func TestRegistry_aGitHubWebhook_readsBackWithItsSourceAndEvents(t *testing.T) {
	r := openRegistry(t)
	ctx := t.Context()
	s := published(t, strings.Replace(definition,
		`  - { type: cron, schedule: "*/15 * * * *" }`,
		`  - { type: webhook, path: gh/triage, from: github, events: [issues.labeled] }`, 1))
	if err := r.Publish(ctx, s, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got, err := r.Get(ctx, "triage", s.Version)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got.Triggers, s.Triggers) {
		t.Errorf("Get triggers = %+v, want %+v", got.Triggers, s.Triggers)
	}
	versions, err := r.Versions(ctx, "triage")
	if err != nil || len(versions) == 0 {
		t.Fatalf("Versions = %v (%v)", versions, err)
	}
	if tr := versions[0].Triggers; len(tr) != 1 || tr[0].From != domain.WebhookFromGitHub ||
		!reflect.DeepEqual(tr[0].Events, []string{"issues.labeled"}) {
		t.Errorf("Versions triggers = %+v, want the source and events", tr)
	}
}

/*
Only the shape of an event name is checked, and that is the contract.

GitHub's events and actions are many and grow; a copy of the list here would
refuse a new event on the day GitHub adds it. So a well-shaped misspelling
publishes — and never fires, which the manual and the Studio say, pointing at
GitHub's delivery log.
*/
func TestParse_aWellShapedMisspelling_publishes(t *testing.T) {
	t.Parallel()

	s := parse(t, withTrigger(`  - { type: webhook, path: a/b, from: github, events: [pull_requset.opened] }`))

	if got := spec.Webhooks(s)[0].Events; !reflect.DeepEqual(got, []string{"pull_requset.opened"}) {
		t.Errorf("events = %v, want the misspelling kept as declared", got)
	}
}

/*
What the door could not act on is refused at parse.

Each of these publishes cleanly if let through and then fires nothing, or fires
on everything: a source nobody serves, events on a hook that cannot filter
them, a GitHub hook that names no event, or a name not shaped like an event.
*/
func TestParse_aGitHubWebhookItCannotServe_isRefused(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"unknown source":                 `  - { type: webhook, path: a/b, from: gitlab, events: [push] }`,
		"events without a source":        `  - { type: webhook, path: a/b, events: [push] }`,
		"github with no event":           `  - { type: webhook, path: a/b, from: github }`,
		"not the shape of an event name": `  - { type: webhook, path: a/b, from: github, events: ["Pull Request"] }`,
		"an action with no event":        `  - { type: webhook, path: a/b, from: github, events: [.opened] }`,
		"ping, which never starts a run": `  - { type: webhook, path: a/b, from: github, events: [ping] }`,
		"a source on a cron":             `  - { type: cron, schedule: "* * * * *", from: github, events: [push] }`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := spec.Parse("test.agent.md", []byte(withTrigger(line)))
			if !errors.Is(err, spec.ErrInvalid) {
				t.Fatalf("err = %v, want the trigger refused", err)
			}
		})
	}
}
