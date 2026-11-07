package regression

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

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
