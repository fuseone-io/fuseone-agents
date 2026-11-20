package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
AppendIfHeadWithContent seals, onto head, the step build returns for data kept as the run's
content at seq, and keeps data in the same transaction: both or neither (NT-018 phase 4).

build is handed the content's reference and digest, so the step names exactly what is
kept. A head that moved, a tombstone at that reference, or a crash between the two writes
leaves nothing behind: content kept first and named after outlived every refused step and
every interrupted one, owned by a run that never pointed at it.
*/
func (p *Postgres) AppendIfHeadWithContent(
	ctx context.Context, head domain.StepRef, run domain.RunID, seq int64, data []byte,
	build func(ref, digest string) domain.Step,
) (domain.Step, error) {
	s, err := contentStep(run, seq, data, build)
	if err != nil {
		return domain.Step{}, err
	}
	var last error
	for range appendRetries {
		sealed, err := p.appendWithContentOnce(ctx, head, s, seq, data)
		if !errors.Is(err, ErrSeqConflict) {
			return sealed, err
		}
		last = err
	}
	return domain.Step{}, fmt.Errorf("append after %d attempts: %w", appendRetries, last)
}

func (p *Postgres) appendWithContentOnce(
	ctx context.Context, head domain.StepRef, s domain.Step, seq int64, data []byte,
) (domain.Step, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Step{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The append first: it takes the run's lock and judges the head, so a refused step
	// keeps nothing, and two keepers of one wait are serialised before either writes.
	sealed, err := appendTx(ctx, tx, nil, &head, s)
	if err != nil {
		return domain.Step{}, err
	}
	if _, err := keepExactTx(ctx, tx, "run", string(s.RunID), seq, data); err != nil {
		return domain.Step{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Step{}, fmt.Errorf("commit: %w", err)
	}
	return sealed, nil
}

// KeptContent reads content a step names, as the content store does.
func (p *Postgres) KeptContent(ctx context.Context, ref string) ([]byte, error) {
	return NewContent(p.pool).Get(ctx, ref)
}

// contentStep is the step build returns for data, held to name content of its own run.
func contentStep(run domain.RunID, seq int64, data []byte, build func(ref, digest string) domain.Step) (domain.Step, error) {
	ref, digest := contentRef("run", string(run), seq, data)
	s := build(ref, digest)
	if s.RunID != run {
		return domain.Step{}, fmt.Errorf("ledger: a step of %s cannot name content of %s", s.RunID, run)
	}
	return s, nil
}

// AppendIfHeadWithContent is the Postgres rule in memory: the step and the content are
// kept together, under the lock, or neither is.
func (m *Memory) AppendIfHeadWithContent(
	ctx context.Context, head domain.StepRef, run domain.RunID, seq int64, data []byte,
	build func(ref, digest string) domain.Step,
) (domain.Step, error) {
	s, err := contentStep(run, seq, data, build)
	if err != nil {
		return domain.Step{}, err
	}
	ref, _ := contentRef("run", string(run), seq, data)
	return m.appendKeeping(ctx, &head, s, ref, append([]byte(nil), data...))
}

// KeptContent reads content a step names.
func (m *Memory) KeptContent(_ context.Context, ref string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.kept[ref]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoContent, ref)
	}
	return append([]byte(nil), data...), nil
}

// appendKeeping seals s onto expect and keeps data under ref in the same critical
// section, so a refused step keeps nothing (append_content.go).
func (m *Memory) appendKeeping(
	ctx context.Context, expect *domain.StepRef, s domain.Step, ref string, data []byte,
) (domain.Step, error) {
	if err := ctx.Err(); err != nil {
		return domain.Step{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	sealed, err := m.appendLocked(nil, expect, s)
	if err != nil {
		return domain.Step{}, err
	}
	if m.kept == nil {
		m.kept = make(map[string][]byte)
	}
	m.kept[ref] = data
	return sealed, nil
}
