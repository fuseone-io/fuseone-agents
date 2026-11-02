package ledger

import (
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
The in-memory link judgement, made under m.mu — the serialisation the Postgres
store buys with its advisory locks. Callers hold m.mu.

Nothing is kept beside the steps: the company's links are read from the sealed
work_item_linked steps of every real run, which is the same set the Postgres
projection holds, because each of them passed this fence as it was sealed.
*/
func (m *Memory) judgeWorkItemLink(s domain.Step) error {
	steps := m.runs[s.RunID]
	p, err := decodeLink(s, int64(len(steps))+domain.FirstSeq)
	if err != nil {
		return err
	}
	agent, scope, err := startedIdentity(steps)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrWorkItemLinkInvalid, err)
	}
	if err := linkSpeaksForRun(s, agent, scope); err != nil {
		return err
	}
	held, _, _ := claimOfRun(steps)
	if err := linkRootHeld(p, held); err != nil {
		return err
	}
	if err := judgeLinkProof(p, memoryStepAt(steps)); err != nil {
		return err
	}
	// A simulation rehearses the link inside its own run and is judged
	// against nobody else's: it projects nothing, so it can conflict with
	// nothing.
	if simulatedRun(steps) {
		return nil
	}
	links := aliasesIn(m.aliasRows(), scope.Company)
	return domain.JudgeAlias(p, links.root[p.Alias], links.hasAliases[p.Alias], links.root[p.Root] != "")
}

func memoryStepAt(steps []domain.Step) stepAt {
	return func(seq int64) (domain.Step, bool, error) {
		if seq < domain.FirstSeq || seq > int64(len(steps)) {
			return domain.Step{}, false, nil
		}
		return steps[seq-domain.FirstSeq], true, nil
	}
}

// aliasRow is one link as the projection holds it: the alias, its root, and
// the linking run's own company and area.
type aliasRow struct {
	scope domain.Scope
	ref   domain.WorkItemRef
	root  domain.WorkItemRef
}

// aliasRows is every link a real run sealed, the first per company and alias.
func (m *Memory) aliasRows() []aliasRow {
	seen := map[aliasKey]bool{}
	var out []aliasRow
	for _, steps := range m.runs {
		if simulatedRun(steps) {
			continue
		}
		scope := identityOf(steps).Scope
		for _, st := range steps {
			if st.Kind != domain.StepWorkItemLinked {
				continue
			}
			var p domain.WorkItemLinkedPayload
			if json.Unmarshal(st.Payload, &p) != nil || seen[aliasKey{scope.Company, p.Alias}] {
				continue
			}
			seen[aliasKey{scope.Company, p.Alias}] = true
			out = append(out, aliasRow{scope: scope, ref: p.Alias, root: p.Root})
		}
	}
	return out
}

type aliasKey struct {
	company domain.CompanyID
	ref     domain.WorkItemRef
}

// companyAliases is one company's links: each alias's root, and which
// references are roots of some alias.
type companyAliases struct {
	root       map[domain.WorkItemRef]domain.WorkItemRef
	hasAliases map[domain.WorkItemRef]bool
}

func aliasesIn(rows []aliasRow, company domain.CompanyID) companyAliases {
	out := companyAliases{root: map[domain.WorkItemRef]domain.WorkItemRef{}, hasAliases: map[domain.WorkItemRef]bool{}}
	for _, r := range rows {
		if r.scope.Company == company {
			out.root[r.ref] = r.root
			out.hasAliases[r.root] = true
		}
	}
	return out
}
