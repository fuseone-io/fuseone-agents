package spec_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
What the activation route reads before and after the locks (NT-018 phase 2, release 2c).

Before any lock, the version's scope — from its published row and its seals, which name
the same bytes — so the caller is authorised on it, and the definition the publication's
checks run over: the published row, else a readable seal. After the commit, the tag the
agent has now, judged under the lock that wrote it.
*/

func TestActivate_answersTheTagTheAgentHasAfterIt(t *testing.T) {
	for _, retired := range []bool{false, true} {
		w := newActivationWorld(t)
		first := w.publishFirst(t)
		_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
		if retired {
			if _, err := w.pool.Exec(t.Context(),
				`update agent_state set retired_at = now() where agent_id = 'triage'`); err != nil {
				t.Fatal(err)
			}
		}
		a := w.activation(t, candidate.Version, first.Version)
		a.Guard.Expect = ifMatch(t, domain.AgentRevision(first.Version, retired))

		got, err := w.publisher.Activate(t.Context(), a)
		if err != nil || got.Revision != domain.AgentRevision(candidate.Version, retired) {
			t.Errorf("retired %v: Activate = %+v, %v; want the candidate's tag", retired, got.Revision, err)
		}
	}
}

func TestActivations_scopes_areThoseOfThePublishedRowAndTheSeals(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	_, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	activations := spec.NewActivations(w.publisher, w.reader)

	for _, c := range []struct {
		version domain.VersionID
		want    int
	}{{first.Version, 1}, {candidate.Version, 1}, {"v000000000000", 0}} {
		scopes, err := activations.Scopes(t.Context(), "triage", c.version)
		if err != nil || len(scopes) != c.want || (c.want == 1 && scopes[0] != cx) {
			t.Errorf("Scopes(%s) = %v, %v; want %d scope(s) in cx", c.version, scopes, err, c.want)
		}
	}
}

func TestActivations_definition_isThePublishedRow_elseAReadableSeal_elseNone(t *testing.T) {
	w := newActivationWorld(t)
	first := w.publishFirst(t)
	seal, candidate := w.seal(t, candidateOf("Read the sealed ticket and classify it."))
	activations := spec.NewActivations(w.publisher, w.reader)

	row, found, err := activations.Definition(t.Context(), "triage", first.Version, cx)
	if err != nil || !found || row.Instructions != first.Instructions {
		t.Errorf("Definition(published) = %q, %v, %v; want its row", row.Instructions, found, err)
	}
	sealed, found, err := activations.Definition(t.Context(), "triage", candidate.Version, cx)
	if err != nil || !found || sealed.Digest != candidate.Digest {
		t.Errorf("Definition(candidate) = %q, %v, %v; want the sealed bytes", sealed.Digest, found, err)
	}
	w.erase(t, seal)
	if _, found, err := activations.Definition(t.Context(), "triage", candidate.Version, cx); err != nil || found {
		t.Errorf("Definition(erased) found %v, %v; want none, for the activation to refuse", found, err)
	}
}
