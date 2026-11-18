package spec_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
A publication's own condition, judged in its transaction (NT-018, contract
4): Within runs once the version is current and before the commit, under the
event graph's lock and the agent's row, so a caller checks and writes what
must move with the version — a workbench draft's ETag and base — or undoes
the publication by refusing.
*/
func TestPublish_within_seesTheVersionCurrentAndItsRefusalUndoesThePublication(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	publisher := spec.NewPublisher(pool, fixedClock{})
	first := published(t, definition)

	refused := errors.New("the draft moved")
	var seen domain.VersionID
	_, err := publisher.Publish(ctx, spec.Publication{Spec: first, By: "usr_ana", Company: "acme",
		Within: func(ctx context.Context, tx pgx.Tx, version domain.VersionID) error {
			current, found, err := spec.CurrentIn(ctx, tx, first.ID)
			if err != nil || !found {
				return errors.Join(err, errors.New("no current version inside the publication"))
			}
			seen = current.Version
			return refused
		}})
	if !errors.Is(err, refused) || seen != version(first) {
		t.Fatalf("Publish = %v, saw %q; want the refusal, the version current inside", err, seen)
	}
	if versions, err := spec.NewRegistry(pool).Versions(ctx, first.ID); err != nil || len(versions) != 0 {
		t.Errorf("versions after a refusal = %+v (%v), want none", versions, err)
	}
}

func version(s spec.Spec) domain.VersionID { return s.Version }
