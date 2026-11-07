package regression

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// ErrExpectationsTooLarge means a case's expectations would not fit in one object.
// Content past the limit is cut, and a cut object would never decode again.
var ErrExpectationsTooLarge = errors.New("regression: these expectations are too large to keep")

const (
	defaultDataset = "default"
	// expectationsKind owns expectations as content. Not the copies' own kind, because
	// nothing that searches copies for an input should ever find one of these.
	expectationsKind = "expectations"
)

func defaultDatasetOf(agent domain.AgentID) string {
	return "default:" + string(agent)
}

/*
write runs one change to the working set, in the transaction that decides how.

The switch is read holding its row, and after any wait for it. Activation holds it
exclusively, so a writer in flight finishes first and the next one reads it on. Read
without the lock, a writer arriving during activation would see it off and write what
the trigger refuses. Declaring itself is what gets a version-aware writer past that
trigger, and it lasts as long as this transaction.
*/
func (s *Store) write(ctx context.Context, change func(tx pgx.Tx, versioned bool) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("regression: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var versioned bool
	if err := tx.QueryRow(ctx,
		`select active from dataset_versioning where only_row for share`).Scan(&versioned); err != nil {
		return fmt.Errorf("regression: read the dataset switch: %w", err)
	}
	if versioned {
		if _, err := tx.Exec(ctx,
			`select set_config('fuseone.dataset_writer', 'version-aware', true)`); err != nil {
			return fmt.Errorf("regression: declare this writer: %w", err)
		}
	}
	if err := change(tx, versioned); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("regression: commit: %w", err)
	}
	return nil
}

func (s *Store) recordVersioned(
	ctx context.Context, tx pgx.Tx, c domain.RegressionCase, expectations []byte,
) error {
	if err := lockDataset(ctx, tx, c.Agent, c.Scope); err != nil {
		return err
	}
	ref, err := s.keepExpectations(ctx, tx, c, expectations)
	if err != nil {
		return err
	}
	if err := upsertCase(ctx, tx, caseRow{c, expectations, &ref}); err != nil {
		return err
	}
	return writeVersion(ctx, tx, c.Agent, c.CreatedBy)
}

func deleteVersioned(
	ctx context.Context, tx pgx.Tx, agent domain.AgentID, id string, by domain.UserID,
) error {
	var locked string
	err := tx.QueryRow(ctx, `select dataset_id from datasets where dataset_id = $1 for update`,
		defaultDatasetOf(agent)).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing was ever recorded here, so there is nothing to delete or to version.
		return nil
	}
	if err != nil {
		return fmt.Errorf("regression: lock the dataset of %s: %w", agent, err)
	}
	if _, err := tx.Exec(ctx, `delete from regression_cases where agent_id = $1 and case_id = $2`,
		string(agent), id); err != nil {
		return fmt.Errorf("regression: delete %s: %w", id, err)
	}
	return writeVersion(ctx, tx, agent, by)
}

// lockDataset makes an agent's writers take turns, so version numbers have no gaps
// and each version is built on the one before.
func lockDataset(ctx context.Context, tx pgx.Tx, agent domain.AgentID, scope domain.Scope) error {
	if _, err := tx.Exec(ctx, `
		insert into datasets (dataset_id, agent_id, name, company_id, area_id)
		values ($1, $2, $3, $4, $5)
		on conflict (dataset_id) do update set name = datasets.name`,
		defaultDatasetOf(agent), string(agent), defaultDataset,
		string(scope.Company), string(scope.Area)); err != nil {
		return fmt.Errorf("regression: lock the dataset of %s: %w", agent, err)
	}
	return nil
}

// expectationsObject names the case, so that identical expectations on two cases are
// two objects and erasing one does not leave the other as a gap.
type expectationsObject struct {
	Case         string          `json:"case"`
	Expectations json.RawMessage `json:"expectations"`
}

/*
keepExpectations stores what a case expects as content, and says where.

The same expectations keep the object they have. Activation wrote that one from jsonb,
whose bytes differ from these, and a new object would report a change nobody made.

In this transaction rather than on the side: an object the failed write never came
to name would sit there readable, with nothing to lead an erasure to it.
*/
func (s *Store) keepExpectations(
	ctx context.Context, tx pgx.Tx, c domain.RegressionCase, expectations []byte,
) (string, error) {
	var ref string
	err := tx.QueryRow(ctx, `
		select c.expectations_ref from regression_cases c
		join run_content ex on ex.ref = c.expectations_ref
		where c.agent_id = $1 and c.case_id = $2 and c.expectations = $3::jsonb
		  and ex.erased_at is null`, string(c.Agent), c.ID, string(expectations)).Scan(&ref)
	if err == nil {
		return ref, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("regression: read the expectations of %s: %w", c.ID, err)
	}

	object, err := json.Marshal(expectationsObject{Case: c.ID, Expectations: expectations})
	if err != nil {
		return "", fmt.Errorf("regression: encode the expectations of %s: %w", c.ID, err)
	}
	if len(object) > domain.DefaultContentLimit {
		return "", fmt.Errorf("%w: %d bytes", ErrExpectationsTooLarge, len(object))
	}
	return s.content.RestoreForTx(ctx, tx, expectationsKind, string(c.Agent), 1, object)
}

// heldExpectations is what one case's row and object say, as List read them.
type heldExpectations struct {
	column []byte
	ref    *string
	found  bool
	object []byte
	erased bool
}

// read prefers the object. A tombstone is an answer and never a reason to fall back,
// which would bring back what somebody erased. Only where no reference exists does the
// column answer, and a reference to nothing refuses, like an unreadable occurrence.
func (h heldExpectations) read() ([]domain.Expectation, error) {
	out := []domain.Expectation{}
	switch {
	case h.ref == nil:
		return out, json.Unmarshal(h.column, &out)
	case !h.found:
		return nil, fmt.Errorf("%s is nowhere", *h.ref)
	case h.erased:
		return out, nil
	}
	var object expectationsObject
	if err := json.Unmarshal(h.object, &object); err != nil {
		return nil, err
	}
	return out, json.Unmarshal(object.Expectations, &out)
}

// versionLine is one case as a version's digest sees it.
type versionLine struct {
	CaseID, InputDigest, ExpectationsDigest string
}

// versionDigest covers the ordered cases, their input digests and their expectation
// digests. Digests and not bytes, so erasing content does not change what a version
// is. The id is length-prefixed, so no id can pass for two.
//
// The activation computes the same thing in SQL, ordering by the bytes of the id.
func versionDigest(lines []versionLine) string {
	sum := sha256.New()
	for _, l := range lines {
		fmt.Fprintf(sum, "%d:%s %s %s\n", len(l.CaseID), l.CaseID, l.InputDigest, l.ExpectationsDigest)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

const versionSourceSQL = `
	from regression_cases c
	left join run_content i on i.ref = c.input_ref
	join run_content e on e.ref = c.expectations_ref
	where c.agent_id = $1`

// writeVersion records the working set as a new version, if it is one. The dataset is
// locked by now. Called by a writer that changed nothing, it writes nothing.
func writeVersion(ctx context.Context, tx pgx.Tx, agent domain.AgentID, by domain.UserID) error {
	rows, err := tx.Query(ctx, `
		select c.case_id, coalesce(i.digest, c.input_ref), e.digest`+versionSourceSQL+`
		order by c.case_id collate "C"`, string(agent))
	if err != nil {
		return fmt.Errorf("regression: read the corpus of %s: %w", agent, err)
	}
	lines, err := pgx.CollectRows(rows, pgx.RowToStructByPos[versionLine])
	if err != nil {
		return fmt.Errorf("regression: read the corpus of %s: %w", agent, err)
	}
	digest := versionDigest(lines)

	dataset := defaultDatasetOf(agent)
	var version int64
	var latest string
	err = tx.QueryRow(ctx, `
		select version, digest from dataset_versions where dataset_id = $1
		order by version desc limit 1`, dataset).Scan(&version, &latest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("regression: read the latest version of %s: %w", agent, err)
	}
	if err == nil && latest == digest {
		return nil
	}

	return insertVersion(ctx, tx, newVersion{agent, version + 1, digest, by})
}

type newVersion struct {
	agent   domain.AgentID
	version int64
	digest  string
	by      domain.UserID
}

func insertVersion(ctx context.Context, tx pgx.Tx, v newVersion) error {
	dataset := defaultDatasetOf(v.agent)
	if _, err := tx.Exec(ctx, `
		insert into dataset_versions (dataset_id, version, digest, created_by)
		values ($1, $2, $3, $4)`, dataset, v.version, v.digest, string(v.by)); err != nil {
		return fmt.Errorf("regression: write a version of %s: %w", v.agent, err)
	}
	if _, err := tx.Exec(ctx, `
		insert into dataset_version_cases
			(dataset_id, version, case_id, input_ref, input_digest,
			 expectations_ref, expectations_digest, from_run)
		select $2, $3::bigint, c.case_id, c.input_ref, coalesce(i.digest, c.input_ref),
		       c.expectations_ref, e.digest, c.from_run`+versionSourceSQL,
		string(v.agent), dataset, v.version); err != nil {
		return fmt.Errorf("regression: write the cases of a version of %s: %w", v.agent, err)
	}
	return nil
}
