package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A step and the content it names, kept together or not at all (NT-018 phase 4).

An author's answer for the world is named by a world_supplied step. Kept first and named
after, an answer whose step lost the race to the head stayed behind with nothing naming
it, and an answer a crash cut off between the two did too. Kept in the transaction that
seals the step, onto the head it was judged against, there is never one without the other.
*/

// contentKeeper is the surface under test, beside the rest of the store.
type contentKeeper interface {
	AppendIfHeadWithContent(
		ctx context.Context, head domain.StepRef, run domain.RunID, seq int64, data []byte,
		build func(ref, digest string) domain.Step,
	) (domain.Step, error)
	KeptContent(ctx context.Context, ref string) ([]byte, error)
}

func keeper(t *testing.T, s Store) contentKeeper {
	t.Helper()
	k, ok := s.(contentKeeper)
	if !ok {
		t.Fatalf("%T keeps no content with its steps", s)
	}
	return k
}

// answering builds the step that names the content, the way the answer route does.
func answering(t *testing.T) func(ref, digest string) domain.Step {
	return func(ref, digest string) domain.Step {
		s := step("run-1", domain.StepWorldSupplied)
		s.Payload = mustJSON(t, domain.WorldSuppliedPayload{AtSeq: 2, Ref: ref, Digest: digest, By: "usr_ana"})
		return s
	}
}

// fresh is bytes no earlier run of this suite kept: the harness truncates the ledger
// between tests, not the content store, and a reference names its bytes.
func fresh(what string) []byte {
	return []byte(fmt.Sprintf(`{"%s":%d}`, what, time.Now().UnixNano()))
}

func TestAppendIfHeadWithContent(t *testing.T) {
	run(t, "onto the expected head, the step is sealed and names the content it kept", func(t *testing.T, s Store) {
		ctx := context.Background()
		head := seedApprovalRequest(t, s)
		var named string
		data := fresh("kept")
		sealed, err := keeper(t, s).AppendIfHeadWithContent(ctx, domain.StepRef{Seq: head.Seq, Kind: head.Kind},
			"run-1", head.Seq+1, data, func(ref, digest string) domain.Step {
				named = ref
				return answering(t)(ref, digest)
			})
		if err != nil || sealed.Seq != head.Seq+1 {
			t.Fatalf("AppendIfHeadWithContent = %d (%v), want it sealed onto the head", sealed.Seq, err)
		}
		kept, err := keeper(t, s).KeptContent(ctx, named)
		if err != nil || string(kept) != string(data) {
			t.Errorf("kept %q (%v), want the bytes the step names", kept, err)
		}
	})

	run(t, "the head moved, so neither the step nor the content is kept", func(t *testing.T, s Store) {
		ctx := context.Background()
		head := seedApprovalRequest(t, s)
		if _, err := s.Append(ctx, step("run-1", domain.StepAbandoned)); err != nil {
			t.Fatal(err)
		}
		var named string
		_, err := keeper(t, s).AppendIfHeadWithContent(ctx, domain.StepRef{Seq: head.Seq, Kind: head.Kind},
			"run-1", head.Seq+1, fresh("refused"), func(ref, digest string) domain.Step {
				named = ref
				return answering(t)(ref, digest)
			})
		if !errors.Is(err, domain.ErrHeadMoved) {
			t.Fatalf("err = %v, want ErrHeadMoved", err)
		}
		if _, err := keeper(t, s).KeptContent(ctx, named); !errors.Is(err, domain.ErrContentAbsent) {
			t.Errorf("the content of a step that was refused: %v, want absent", err)
		}
		steps, _ := s.Read(ctx, "run-1", domain.FirstSeq)
		if last := steps[len(steps)-1]; last.Kind != domain.StepAbandoned {
			t.Errorf("the run ends in %s, want nothing after the move", last.Kind)
		}
	})

	run(t, "a step for another run than the content's owner is refused", func(t *testing.T, s Store) {
		ctx := context.Background()
		head := seedApprovalRequest(t, s)
		_, err := keeper(t, s).AppendIfHeadWithContent(ctx, domain.StepRef{Seq: head.Seq, Kind: head.Kind},
			"run-2", head.Seq+1, []byte(`{}`), answering(t))
		if err == nil {
			t.Fatal("content owned by one run was named by a step of another")
		}
	})
}

// An erasure leaves a tombstone, and the same bytes at the same place never bring the
// content back: the step is refused with the content.
func TestAppendIfHeadWithContent_overATombstone_keepsNothing(t *testing.T) {
	pool := testdb.Isolated(t)
	ctx := context.Background()
	store := ledger.NewPostgres(pool)
	head := seedApprovalRequest(t, store)
	data := fresh("erased")
	ref, err := ledger.NewContent(pool).Put(ctx, "run-1", head.Seq+1, data)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.NewContent(pool).EraseRefsTx(ctx, tx, []string{ref}, "subject"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = store.AppendIfHeadWithContent(ctx, domain.StepRef{Seq: head.Seq, Kind: head.Kind},
		"run-1", head.Seq+1, data, answering(t))
	if !errors.Is(err, domain.ErrContentErased) {
		t.Fatalf("err = %v, want ErrContentErased", err)
	}
	steps, _ := store.Read(ctx, "run-1", domain.FirstSeq)
	if len(steps) != 2 {
		t.Errorf("a step was sealed over erased content: %d steps", len(steps))
	}
}
