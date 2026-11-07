package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// eraseExpectationsOf takes what the corrections holding these copies or made from these
// runs expected, through the versions naming them. Before the copies, on purpose: taking
// a copy brings its objects with it, by trigger, and the event would not count them. A
// case whose copy was never stored has no tombstone for that trigger to follow anyway.
func (e *Erasures) eraseExpectationsOf(
	ctx context.Context, tx pgx.Tx, copies, runs []string,
) (int, error) {
	refs, err := scanStrings(ctx, tx, `
		select expectations_ref from dataset_version_cases where input_ref = any($1::text[])
		union
		select expectations_ref from dataset_version_cases where from_run = any($2::text[])`, copies, runs)
	if err != nil {
		return 0, fmt.Errorf("admin: find what corrections expected: %w", err)
	}
	objects, err := e.content.EraseRefsTx(ctx, tx, refs, "subject")
	if err != nil {
		return 0, fmt.Errorf("admin: erase what corrections expected: %w", err)
	}
	return objects, nil
}

// eraseExpectationsOfExpired is retention's half, and it runs before the copies go for
// the same reason. An object can be younger than the copy it describes, and it goes with it.
func eraseExpectationsOfExpired(ctx context.Context, tx pgx.Tx, before time.Time) (int, error) {
	tag, err := tx.Exec(ctx, `
		update run_content held
		set bytes = null, erased_at = now(), erased_reason = 'retention'
		from dataset_version_cases v
		join run_content copied on copied.ref = v.input_ref
		where copied.created_at < $1 and copied.erased_at is null
		  and held.ref = v.expectations_ref and held.erased_at is null`, before.UTC())
	if err != nil {
		return 0, fmt.Errorf("admin: erase what expired corrections expected: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
