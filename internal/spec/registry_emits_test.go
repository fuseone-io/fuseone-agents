package spec_test

import (
	"context"
	"strings"
	"testing"
)

/*
What a version declares it publishes is read from that version, not from
whichever is current (#203). The dispatcher asks it of the version a
finished run was pinned to — the one the engine resolved the run from — so a
publish in the middle of a run changes neither the fan-out nor the run's own
withheld record.
*/
func TestEmitsOf_readsThePinnedVersion_notTheLatest(t *testing.T) {
	emitting := strings.Replace(definition, "tools:", "emits:\n  - ticket.triado\ntools:", 1)
	silent := strings.Replace(definition, "Read the ticket", "Read the ticket again", 1)
	cases := map[string]struct{ first, second string }{
		"emitted, then stopped": {emitting, silent},
		"silent, then emits":    {silent, emitting},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			registry := openRegistry(t)
			ctx := context.Background()
			first, second := published(t, c.first), published(t, c.second)
			if err := registry.Publish(ctx, first, "usr_ana", "acme"); err != nil {
				t.Fatalf("Publish first: %v", err)
			}
			if err := registry.Publish(ctx, second, "usr_ana", "acme"); err != nil {
				t.Fatalf("Publish second: %v", err)
			}

			got, err := registry.EmitsOf(ctx, first.ID, first.Version)
			if err != nil {
				t.Fatalf("EmitsOf: %v", err)
			}
			if want := len(first.Emits); len(got) != want {
				t.Fatalf("EmitsOf(first) = %+v, want the first version's %d events", got, want)
			}
		})
	}
}
