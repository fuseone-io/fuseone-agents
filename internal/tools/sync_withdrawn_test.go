package tools_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A ruling withdrawn stops deciding the tool (NT-019).

Rulings used to be only ever replaced, so a Sync that applied what it read and
left everything else alone was enough. The platform's ceiling can now be
withdrawn: with no owner's ruling under it the tool has no ruling at all, and
must read as discovery leaves a tool, not keep the withdrawn ceiling's effect
in every running worker until it restarts.
*/
func TestSync_aToolWhoseRulingWasWithdrawn_readsUnclassifiedAgain(t *testing.T) {
	t.Parallel()
	c, _ := catalogWith(t, noteServer())
	if _, err := c.Sync(t.Context(), staticRulings{{
		Tool: "crm.note", Effect: domain.EffectWrite, Untrusted: false, CompensatedBy: "crm.unnote",
		Dedupe: domain.ToolDedupe{WindowSeconds: 60, ArgPaths: []string{"text"}},
	}}, domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if effect, _ := c.Effect("crm.note"); effect != domain.EffectWrite {
		t.Fatalf("effect = %v, want the ruling applied first", effect)
	}

	if _, err := c.Sync(t.Context(), staticRulings{}, domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if effect, _ := c.Effect("crm.note"); effect != domain.EffectUnknown {
		t.Errorf("effect = %v, want unknown once no ruling decides it", effect)
	}
	if undo, ok := c.CompensatedBy("crm.note"); ok {
		t.Errorf("compensated by %s, want nothing left of the withdrawn ruling", undo)
	}
	if _, ok := c.Dedupe("crm.note"); ok {
		t.Error("the withdrawn ruling's dedupe still applies")
	}
}
