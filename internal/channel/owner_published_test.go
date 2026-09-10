package channel_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

const asking = `---
id: cobranca
name: Cobrança
area: ops
provider: openai
model: test-model
tools: [crm.lookup]
approvals:
  direct: true
budget: {micros: 500000, steps: 60}
triggers:
  - { type: cron, schedule: "*/15 * * * *" }
---

Cobre com educação.
`

/*
What an owner wrote in the file is what the sweep obeys.

Every other test here hands the reporter a policy directly, so all of them would
stay green if the file, the registry and the port disagreed about what the
policy is — and each of those seams has already lost it once: the renderer
dropped it, the API dropped it, and the read accepted a shape publishing
refuses.

So this one starts where an owner starts. It publishes a definition that asks
for a private approval, parks a run pinned to that version, and sweeps.
*/
func TestSweep_anAgentPublishedAskingPrivately_reachesTheDecider(t *testing.T) {
	store, pool := channelStore(t)
	registry := spec.NewRegistry(pool)
	ctx := context.Background()

	published, err := spec.Parse("cobranca.agent.md", []byte(asking))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := registry.Publish(ctx, published, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	scope := domain.Scope{Company: "acme", Area: "ops"}
	awaitApprovalOf(t, pool, scope, "run-published", "cobranca", published.Version)

	posts := &recorder{}
	r := channel.NewReporter(store, rooms(), posts,
		func() time.Time { return time.Now() }, nil).
		WithDeliveries(store).
		WithDirectApprovals(deciders("usr_ana"),
			accountBook{"acme-slack": {"usr_ana": "U-ana"}}).
		// The real registry, answering for the version the run pinned.
		WithOwnerApprovals(registry, oneConnection)

	if _, err := r.Sweep(ctx, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if !addressed(posts.sent, "U-ana") {
		t.Fatalf("sent %+v, want the decider told privately", posts.sent)
	}
}

// And a definition that asks for nothing reaches nobody, through the same
// pieces: the default is the console alone, and it survives publication.
func TestSweep_anAgentPublishedAskingNothing_reachesNobody(t *testing.T) {
	store, pool := channelStore(t)
	registry := spec.NewRegistry(pool)
	ctx := context.Background()

	published, err := spec.Parse("cobranca.agent.md",
		[]byte(strings.Replace(asking, "approvals:\n  direct: true\n", "", 1)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := registry.Publish(ctx, published, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	scope := domain.Scope{Company: "acme", Area: "ops"}
	awaitApprovalOf(t, pool, scope, "run-quiet", "cobranca", published.Version)

	posts := &recorder{}
	r := channel.NewReporter(store, rooms(), posts,
		func() time.Time { return time.Now() }, nil).
		WithDeliveries(store).
		WithDirectApprovals(deciders("usr_ana"),
			accountBook{"acme-slack": {"usr_ana": "U-ana"}}).
		WithOwnerApprovals(registry, oneConnection)

	if _, err := r.Sweep(ctx, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("sent %+v, want nothing", posts.sent)
	}
}
