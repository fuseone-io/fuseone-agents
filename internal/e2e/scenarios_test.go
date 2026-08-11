package e2e_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// A read-only tool call, then an answer. The path every agent takes.
func readThenAnswer(turn int) chatReply {
	if turn == 0 {
		return chatReply{
			Tool: "crm.lookup", Args: `{"email":"cliente@exemplo.com.br"}`,
			PromptTokens: 1200, CompletionTokens: 80,
		}
	}
	return chatReply{Text: "Conta acct_4471, cliente ativo.", PromptTokens: 1400, CompletionTokens: 120}
}

func TestRun_readOnlyAgent_reachesAVerifiableFinish(t *testing.T) {
	eachLedger(t, "a read-only agent runs to a verifiable finish", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, readThenAnswer)
		if err := p.catalog.Classify("crm.lookup", domain.EffectRead, false); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.open(t, "run-e2e-1")

		state := p.settle(t, "run-e2e-1")

		if state.Phase != engine.PhaseFinished {
			t.Fatalf("phase = %v, want finished; steps: %v", state.Phase, kindsOf(p.steps(t, "run-e2e-1")))
		}
		// The chain is the product's central claim. If it does not verify after
		// a real run, nothing else in the audit trail is worth reading.
		if err := store.Verify(t.Context(), "run-e2e-1"); err != nil {
			t.Errorf("Verify: %v", err)
		}
	})
}

func TestRun_theToolActuallyReachesTheMCPServer(t *testing.T) {
	eachLedger(t, "the tool call arrives at the server with the model's arguments", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, readThenAnswer)
		if err := p.catalog.Classify("crm.lookup", domain.EffectRead, false); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.open(t, "run-e2e-2")
		p.settle(t, "run-e2e-2")

		// A ledger full of tool_called steps proves the platform decided to
		// call something. Only the server proves it happened.
		calls := p.server.Lookups()
		if len(calls) != 1 {
			t.Fatalf("MCP server saw %d calls, want exactly 1", len(calls))
		}
		if calls[0].Email != "cliente@exemplo.com.br" {
			t.Errorf("email = %q, want the address the model asked for", calls[0].Email)
		}
	})
}

func TestRun_theTrailRecordsGateThenReserveThenCall(t *testing.T) {
	eachLedger(t, "the trail records the decision before the effect", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, readThenAnswer)
		if err := p.catalog.Classify("crm.lookup", domain.EffectRead, false); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.open(t, "run-e2e-3")
		p.settle(t, "run-e2e-3")

		kinds := kindsOf(p.steps(t, "run-e2e-3"))

		// The order is the guarantee, not an implementation detail: an auditor
		// reading this trail can see that nothing was reserved before the Gate
		// ruled, and nothing was called before it was reserved.
		for _, want := range []domain.StepKind{
			domain.StepRunStarted, domain.StepPlanned, domain.StepGateDecided,
			domain.StepBudgetReserved, domain.StepToolCalled, domain.StepToolReturned,
			domain.StepRunFinished,
		} {
			if !slices.Contains(kinds, want) {
				t.Fatalf("no %s step in the trail: %v", want, kinds)
			}
		}
		if !inOrder(kinds, domain.StepGateDecided, domain.StepBudgetReserved, domain.StepToolCalled) {
			t.Errorf("trail records the effect before the decision: %v", kinds)
		}
	})
}

func TestRun_costIsSpentNotLeftReserved(t *testing.T) {
	eachLedger(t, "a finished run leaves nothing reserved", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, readThenAnswer)
		if err := p.catalog.Classify("crm.lookup", domain.EffectRead, false); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.open(t, "run-e2e-4")

		state := p.settle(t, "run-e2e-4")

		if state.Spent.Tokens == 0 {
			t.Error("the run finished having accounted no tokens at all")
		}
		// Reserve-before, reconcile-after: a reservation still standing on a
		// finished run is budget nobody can ever use again.
		if state.Reserved.Micros != 0 {
			t.Errorf("reserved = %d micros on a finished run, want 0", state.Reserved.Micros)
		}
	})
}

// readThenWrite has the model read a ticket and then try to write a note back
// — the shape of nearly every useful agent, and the one the taint rule governs.
func readThenWrite(turn int) chatReply {
	switch turn {
	case 0:
		return chatReply{
			Tool: "crm.lookup", Args: `{"email":"cliente@exemplo.com.br"}`,
			PromptTokens: 1200, CompletionTokens: 80,
		}
	case 1:
		return chatReply{
			Tool: "crm.note", Args: `{"text":"cobranca duplicada"}`,
			PromptTokens: 1500, CompletionTokens: 90,
		}
	}
	return chatReply{Text: "Nota registrada.", PromptTokens: 1600, CompletionTokens: 40}
}

func TestRun_writeOnDataReadFromOutside_stopsForAHumanBeforeTheWriteHappens(t *testing.T) {
	eachLedger(t, "a write on untrusted data waits for a person", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, readThenWrite)
		// An imported tool is read-only until the Curator says otherwise; this
		// is that act. Reading from an outside system marks the run untrusted.
		if err := p.catalog.Classify("crm.lookup", domain.EffectRead, true); err != nil {
			t.Fatalf("classify lookup: %v", err)
		}
		if err := p.catalog.Classify("crm.note", domain.EffectWrite, false); err != nil {
			t.Fatalf("classify note: %v", err)
		}
		p.open(t, "run-e2e-5")

		state := p.settle(t, "run-e2e-5")

		// The containment claim, checked where it matters: not that the ledger
		// recorded a refusal, but that the CRM never received the note.
		if notes := p.server.Notes(); len(notes) != 0 {
			t.Errorf("the write reached the server before anyone approved it: %v", notes)
		}
		if state.Phase != engine.PhaseAwaitingApproval {
			t.Errorf("phase = %v, want the run waiting on a person", state.Phase)
		}
		if len(p.server.Lookups()) != 1 {
			t.Error("the read that preceded the write did not happen")
		}
	})
}

func TestRun_toolOutsideTheAgentsPack_neverReachesTheServer(t *testing.T) {
	eachLedger(t, "a tool the agent was not granted is refused", func(t *testing.T, store Store) {
		// The model asks for something real and correctly classified. The only
		// thing wrong with it is that this agent's specification never listed
		// it, which is the whole point of a capability pack.
		p := newPlatform(t, store, agentReadOnly, func(int) chatReply {
			return chatReply{Tool: "crm.note", Args: `{"text":"fora do pacote"}`, PromptTokens: 900, CompletionTokens: 30}
		})
		if err := p.catalog.Classify("crm.note", domain.EffectWrite, false); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.open(t, "run-e2e-6")

		state := p.settle(t, "run-e2e-6")

		if notes := p.server.Notes(); len(notes) != 0 {
			t.Errorf("a tool outside the pack was executed: %v", notes)
		}
		if state.Phase == engine.PhaseFinished {
			t.Error("the run finished as though the refused call had succeeded")
		}
	})
}

func inOrder[T comparable](haystack []T, wanted ...T) bool {
	at := -1
	for _, w := range wanted {
		i := slices.Index(haystack, w)
		if i <= at {
			return false
		}
		at = i
	}
	return true
}
