package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
AppendFirstWithInput opens a run: it seals the run_started build returns for input, and keeps
input as the run's content at its first step in the same transaction, both or neither (#432).

Kept first and named after, an input whose opening lost the race for its idempotency key
stayed behind with no run naming it, out of reach of every erasure that starts from a run.
Here the opening judges the key before anything is kept, and a refused one rolls back the
input with it.

The input is kept as an input always was: whole up to the installation's limit and a prefix
beyond it, under the digest of all of it, so an auditor holding the original can still prove
it is the one the run used. A tombstone at that place is a refusal: the same bytes never bring
back an erased input.
*/
func (p *Postgres) AppendFirstWithInput(
	ctx context.Context, run domain.RunID, input []byte, build func(ref, digest string) domain.Step,
) (domain.Step, error) {
	s, err := openingStep(run, input, build)
	if err != nil {
		return domain.Step{}, err
	}
	var last error
	for range appendRetries {
		sealed, err := p.openWithInputOnce(ctx, s, input)
		if !errors.Is(err, ErrSeqConflict) {
			return sealed, err
		}
		last = err
	}
	return domain.Step{}, fmt.Errorf("open after %d attempts: %w", appendRetries, last)
}

func (p *Postgres) openWithInputOnce(ctx context.Context, s domain.Step, input []byte) (domain.Step, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Step{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sealed, err := appendTx(ctx, tx, nil, nil, s)
	if err != nil {
		return domain.Step{}, err
	}
	if err := keepInputTx(ctx, tx, s.RunID, input); err != nil {
		return domain.Step{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Step{}, fmt.Errorf("commit: %w", err)
	}
	return sealed, nil
}

// keepInputTx keeps a run's input at its first step, as Content.Put does, inside tx.
func keepInputTx(ctx context.Context, tx pgx.Tx, run domain.RunID, input []byte) error {
	ref, digest := contentRef("run", string(run), domain.FirstSeq, input)
	stored, truncated := domain.Truncate(input, domain.DefaultContentLimit)
	if _, err := tx.Exec(ctx, `
		insert into run_content
			(ref, owner_kind, run_id, seq, digest, bytes, full_bytes, truncated)
		values ($1, 'run', $2, $3, $4, $5, $6, $7)
		on conflict (ref) do nothing`,
		ref, string(run), domain.FirstSeq, digest, stored, len(input), truncated); err != nil {
		return fmt.Errorf("ledger: keep the input of %s: %w", run, err)
	}
	var erased *time.Time
	if err := tx.QueryRow(ctx, `select erased_at from run_content where ref = $1`, ref).Scan(&erased); err != nil {
		return fmt.Errorf("ledger: read back the input of %s: %w", run, err)
	}
	if erased != nil {
		return fmt.Errorf("%w: %s", ErrErased, ref)
	}
	return nil
}

// openingStep is the step build returns for input: the run's opening, and its own.
func openingStep(run domain.RunID, input []byte, build func(ref, digest string) domain.Step) (domain.Step, error) {
	ref, digest := contentRef("run", string(run), domain.FirstSeq, input)
	s := build(ref, digest)
	if s.RunID != run || s.Kind != domain.StepRunStarted {
		return domain.Step{}, fmt.Errorf("ledger: %s of %s cannot open %s with its input", s.Kind, s.RunID, run)
	}
	return s, nil
}

// InputSink is where a memory ledger also keeps an input, for tests whose readers read
// content from a store of their own. Declared here, by the ledger that writes to it.
type InputSink interface {
	Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error)
}

// WithInputs makes a memory ledger keep each input it opens a run with in sink too.
func (m *Memory) WithInputs(sink InputSink) *Memory {
	m.inputs = sink
	return m
}

// AppendFirstWithInput is the Postgres rule in memory: the opening is judged under the lock,
// and only an opening that was sealed keeps its input.
func (m *Memory) AppendFirstWithInput(
	ctx context.Context, run domain.RunID, input []byte, build func(ref, digest string) domain.Step,
) (domain.Step, error) {
	s, err := openingStep(run, input, build)
	if err != nil {
		return domain.Step{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.Step{}, err
	}
	ref, _ := contentRef("run", string(run), domain.FirstSeq, input)
	m.mu.Lock()
	defer m.mu.Unlock()
	// Judged and sealed first, recorded last: an opening that loses its key keeps no input,
	// and an input that cannot be kept opens no run.
	sealed, err := m.sealLocked(nil, nil, s)
	if err != nil {
		return domain.Step{}, err
	}
	if m.inputs != nil {
		if _, err := m.inputs.Put(ctx, run, domain.FirstSeq, input); err != nil {
			return domain.Step{}, fmt.Errorf("ledger: keep the input of %s: %w", run, err)
		}
	}
	stored, _ := domain.Truncate(input, domain.DefaultContentLimit)
	if m.kept == nil {
		m.kept = make(map[string][]byte)
	}
	m.kept[ref] = append([]byte(nil), stored...)
	m.commitLocked(sealed)
	return sealed, nil
}
