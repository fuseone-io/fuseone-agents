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

// ErrDatasetElsewhere is a case recorded into an agent's corpus from another scope than the
// one the corpus was first recorded in (NT-018 phase 5, D3).
var ErrDatasetElsewhere = errors.New("regression: this corpus is recorded in another scope")

// ErrWorldUnkeepable is a world the replay could not read whole: an answer that names no
// call, or an answer or a world that would be kept only in part. It is refused before it is
// kept, rather than committed and found unreadable by the first battery that replays it.
var ErrWorldUnkeepable = errors.New("regression: this world cannot be kept whole")

// Answer is one answer an author gave in a rehearsal, for the call it answered.
type Answer struct {
	Tool       domain.ToolID
	ArgsDigest string
	Bytes      []byte
}

// WorldCopy is the world a case keeps: the answers, in the order the rehearsal asked.
type WorldCopy struct{ Answers []Answer }

// Copying is what a correction copies into the corpus: what the run was about and, for a
// case kept from a rehearsal, its world. A nil World keeps none.
type Copying struct {
	Input []byte
	World *WorldCopy
}

/*
RecordCopying records a correction and everything it copies in one transaction (NT-018
phase 5b, C2): the input, each answer, the world naming them, the expectations, the case and
the version. A recording refused or failing anywhere keeps none of it: an object kept on the
side, that the failed write never came to name, would sit readable with nothing to lead an
erasure to it. It answers the case as recorded, with the references it kept.

The answers and the world are owned by the case and kept as case_world, never as the copies'
kind: an erasure searching copies for an input by digest must not find an answer that
happens to share its bytes.
*/
func (s *Store) RecordCopying(ctx context.Context, c domain.RegressionCase, copying Copying) (domain.RegressionCase, error) {
	expectations, err := checkedExpectations(c)
	if err != nil {
		return domain.RegressionCase{}, err
	}
	err = s.write(ctx, func(tx pgx.Tx) error {
		if err := lockDataset(ctx, tx, c.Agent, c.Scope); err != nil {
			return err
		}
		if c.InputRef, err = s.content.RestoreForTx(ctx, tx, "regression", string(c.Agent), 1, copying.Input); err != nil {
			return fmt.Errorf("regression: keep the occurrence of %s: %w", c.ID, err)
		}
		if copying.World != nil {
			if c.World, err = s.keepWorld(ctx, tx, c, copying.World); err != nil {
				return err
			}
		}
		return s.recordCase(ctx, tx, c, expectations)
	})
	return c, err
}

// keepWorld keeps each answer, then the world naming them, as the case's own content.
func (s *Store) keepWorld(ctx context.Context, tx pgx.Tx, c domain.RegressionCase, w *WorldCopy) (string, error) {
	owner := string(c.Agent) + "/" + c.ID
	world := domain.CaseWorld{Case: c.ID, Entries: make([]domain.WorldEntry, 0, len(w.Answers))}
	for i, a := range w.Answers {
		if err := keepable(a); err != nil {
			return "", fmt.Errorf("%w: answer %d of %s %v", ErrWorldUnkeepable, i+1, c.ID, err)
		}
		ref, err := s.content.RestoreForTx(ctx, tx, WorldKind, owner, int64(i+2), a.Bytes)
		if err != nil {
			return "", fmt.Errorf("regression: keep answer %d of %s: %w", i+1, c.ID, err)
		}
		sum := sha256.Sum256(a.Bytes)
		world.Entries = append(world.Entries, domain.WorldEntry{
			Position: i + 1, Tool: a.Tool, ArgsDigest: a.ArgsDigest,
			AnswerRef: ref, AnswerDigest: hex.EncodeToString(sum[:]), AnswerBytes: int64(len(a.Bytes)),
		})
	}
	object, err := json.Marshal(world)
	if err != nil {
		return "", fmt.Errorf("regression: encode the world of %s: %w", c.ID, err)
	}
	if len(object) > domain.DefaultContentLimit {
		return "", fmt.Errorf("%w: the world of %s is %d bytes", ErrWorldUnkeepable, c.ID, len(object))
	}
	ref, err := s.content.RestoreForTx(ctx, tx, WorldKind, owner, 1, object)
	if err != nil {
		return "", fmt.Errorf("regression: keep the world of %s: %w", c.ID, err)
	}
	return ref, nil
}

// keepable holds an answer to what the replay's reader requires of it: the call it answers,
// named, and its bytes kept whole.
func keepable(a Answer) error {
	switch {
	case a.Tool == "" || a.ArgsDigest == "":
		return errors.New("names no call")
	case len(a.Bytes) > domain.DefaultContentLimit:
		return fmt.Errorf("is %d bytes, past what is kept whole", len(a.Bytes))
	}
	return nil
}
