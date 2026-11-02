package ledger

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
recordWorkItemLink judges and projects a link inside the append's transaction
(#204, slice 2b). A refusal takes the step with it.

Everything about the run is read before the step reaches the runs projection,
as the claim's judgement is: the run's identity, the card it holds and the
proof steps are its own record, under the run lock the append already holds.

Then, for a real run, lock first and judge second. "One root per alias" and
"one level deep" span runs, so the link takes an advisory lock on each of its
two references in the company — sorted, so two links that share a reference
always queue in the same order and cannot deadlock — and only then reads the
company's links, in a separate statement whose snapshot is taken after the
wait. Every link touching either reference meets one of those locks: two
links of one alias, an alias becoming a root while it is being linked, a root
becoming an alias while it gains one.

Lock order is always the run first, then the references, and a step is either
a claim or a link, so the claim's (agent, card) lock is never held with them.
*/
func recordWorkItemLink(ctx context.Context, tx pgx.Tx, s domain.Step) error {
	p, run, err := judgeLinkInRun(ctx, tx, s)
	if err != nil || run.simulated {
		return err
	}
	return projectLink(ctx, tx, s, p, run)
}

// judgeLinkInRun is the fence against the run's own record: the shape, the
// run's identity, the card it holds and the proof.
func judgeLinkInRun(ctx context.Context, tx pgx.Tx, s domain.Step) (domain.WorkItemLinkedPayload, linkRun, error) {
	p, err := decodeLink(s, s.Seq)
	if err != nil {
		return p, linkRun{}, err
	}
	run, err := linkingRun(ctx, tx, s.RunID)
	if err != nil {
		return p, linkRun{}, err
	}
	if err := linkSpeaksForRun(s, run.agent, run.scope); err != nil {
		return p, linkRun{}, err
	}
	held, err := heldCard(ctx, tx, s, run.startedAt)
	if err != nil {
		return p, linkRun{}, err
	}
	if err := linkRootHeld(p, held); err != nil {
		return p, linkRun{}, err
	}
	return p, run, judgeLinkProof(p, postgresStepAt(ctx, tx, s.RunID, run.startedAt))
}

// projectLink is the fence against the company's other links, under both
// references' locks, and the row it projects.
func projectLink(ctx context.Context, tx pgx.Tx, s domain.Step, p domain.WorkItemLinkedPayload, run linkRun) error {
	if err := lockAliasRefs(ctx, tx, run.scope.Company, p.Alias, p.Root); err != nil {
		return err
	}
	aliasRoot, aliasHasAliases, rootIsAlias, err := companyLinks(ctx, tx, run.scope.Company, p)
	if err != nil {
		return err
	}
	if err := domain.JudgeAlias(p, aliasRoot, aliasHasAliases, rootIsAlias); err != nil {
		return err
	}
	if aliasRoot == p.Root {
		// The same link again: sealed as the run's act, with nothing new to
		// project.
		return nil
	}
	if _, err := tx.Exec(ctx, `
		insert into work_item_aliases (
			company_id, ref, root_ref, area_id, agent_id, run_id, linked_seq, linked_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		string(run.scope.Company), string(p.Alias), string(p.Root), string(run.scope.Area),
		string(run.agent), string(s.RunID), s.Seq, s.At,
	); err != nil {
		return fmt.Errorf("project work item link: %w", err)
	}
	return nil
}

// linkRun is the run as the projection holds it before the link.
type linkRun struct {
	claimRun
	startedAt time.Time
}

func linkingRun(ctx context.Context, tx pgx.Tx, runID domain.RunID) (linkRun, error) {
	r, err := claimingRun(ctx, tx, runID)
	if err != nil {
		return linkRun{}, fmt.Errorf("%w: %w", domain.ErrWorkItemLinkInvalid, err)
	}
	out := linkRun{claimRun: r}
	if err := tx.QueryRow(ctx, `select started_at from runs where run_id = $1`, string(runID)).
		Scan(&out.startedAt); err != nil {
		return linkRun{}, fmt.Errorf("read the linking run: %w", err)
	}
	return out, nil
}

// heldCard is the card the run holds: its projected claim, or for a
// simulation — which projects none — its first claim step.
func heldCard(ctx context.Context, tx pgx.Tx, s domain.Step, openedAt time.Time) (domain.WorkItemRef, error) {
	var held string
	if err := tx.QueryRow(ctx, `
		select coalesce(
			(select ref from run_work_items where run_id = $1),
			(select payload->>'ref' from run_steps
			 where run_id = $1 and opened_at = $2 and kind = 'work_item_claimed' and seq < $3
			 order by seq limit 1),
			'')`,
		string(s.RunID), openedAt, s.Seq,
	).Scan(&held); err != nil {
		return "", fmt.Errorf("read the card the run holds: %w", err)
	}
	return domain.WorkItemRef(held), nil
}

// postgresStepAt reads one step of the run, whole — its labels included —
// in the run's own partition.
func postgresStepAt(ctx context.Context, tx pgx.Tx, runID domain.RunID, openedAt time.Time) stepAt {
	return func(seq int64) (domain.Step, bool, error) {
		rows, err := tx.Query(ctx, `select `+stepColumns+` from run_steps
			where run_id = $1 and seq = $2 and opened_at = $3`,
			string(runID), seq, openedAt)
		if err != nil {
			return domain.Step{}, false, fmt.Errorf("read the link's proof: %w", err)
		}
		steps, err := scanSteps(rows)
		if err != nil {
			return domain.Step{}, false, fmt.Errorf("read the link's proof: %w", err)
		}
		if len(steps) == 0 {
			return domain.Step{}, false, nil
		}
		return steps[0], true, nil
	}
}

// lockAliasRefs takes the company's lock on each reference, in sorted order.
func lockAliasRefs(ctx context.Context, tx pgx.Tx, company domain.CompanyID, refs ...domain.WorkItemRef) error {
	slices.Sort(refs)
	for _, ref := range refs {
		if _, err := tx.Exec(ctx,
			`select pg_advisory_xact_lock(hashtextextended('work_item_alias:' || $1::text || chr(31) || $2::text, 0))`,
			string(company), string(ref),
		); err != nil {
			return fmt.Errorf("lock work item alias: %w", err)
		}
	}
	return nil
}

// companyLinks reads, once both locks are held, what the company's links say
// about the two references.
func companyLinks(
	ctx context.Context, tx pgx.Tx, company domain.CompanyID, p domain.WorkItemLinkedPayload,
) (domain.WorkItemRef, bool, bool, error) {
	var aliasRoot string
	var aliasHasAliases, rootIsAlias bool
	if err := tx.QueryRow(ctx, `
		select
			coalesce((select root_ref from work_item_aliases where company_id = $1 and ref = $2), ''),
			exists (select 1 from work_item_aliases where company_id = $1 and root_ref = $2),
			exists (select 1 from work_item_aliases where company_id = $1 and ref = $3)`,
		string(company), string(p.Alias), string(p.Root),
	).Scan(&aliasRoot, &aliasHasAliases, &rootIsAlias); err != nil {
		return "", false, false, fmt.Errorf("judge work item link: %w", err)
	}
	return domain.WorkItemRef(aliasRoot), aliasHasAliases, rootIsAlias, nil
}
