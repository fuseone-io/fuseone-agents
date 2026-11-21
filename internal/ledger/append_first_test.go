package ledger_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A run's opening and the input it names, kept together or not at all (#432).

Kept first and named after, an input whose opening lost the race for its idempotency key
stayed behind with no run naming it: retention reached it in time, but an erasure that starts
from a run, a data subject's or a conversation's, never could. Kept in the transaction that
opens the run, a refused opening keeps nothing. The input is kept as every input always was:
whole up to the installation's limit and a prefix beyond it, under the digest of all of it.
*/

type inputOpener interface {
	AppendFirstWithInput(ctx context.Context, run domain.RunID, input []byte, build func(ref, digest string) domain.Step) (domain.Step, error)
	KeptContent(ctx context.Context, ref string) ([]byte, error)
}

func opener(t *testing.T, s Store) inputOpener {
	t.Helper()
	o, ok := s.(inputOpener)
	if !ok {
		t.Fatalf("%T cannot open a run with its input", s)
	}
	return o
}

// opening builds the run_started a door writes, naming the input by reference.
func opening(run domain.RunID, key string, named *string) func(ref, digest string) domain.Step {
	return func(ref, _ string) domain.Step {
		*named = ref
		s := step(run, domain.StepRunStarted)
		s.IdemKey = key
		s.Payload = []byte(`{"trigger":"webhook","input_ref":"` + ref + `"}`)
		return s
	}
}

func TestAppendFirstWithInput(t *testing.T) {
	run(t, "the run opens naming the input it kept", func(t *testing.T, s Store) {
		ctx := context.Background()
		input := fresh("ticket")
		var named string
		sealed, err := opener(t, s).AppendFirstWithInput(ctx, "run-1", input, opening("run-1", "hook:1", &named))
		if err != nil || sealed.Seq != domain.FirstSeq || sealed.Kind != domain.StepRunStarted {
			t.Fatalf("AppendFirstWithInput = %d %s (%v), want the run opened", sealed.Seq, sealed.Kind, err)
		}
		if kept, err := opener(t, s).KeptContent(ctx, named); err != nil || !bytes.Equal(kept, input) {
			t.Errorf("kept %q (%v), want the input the opening names", kept, err)
		}
	})

	// The second attempt under one key is another run id, as two attempts at one
	// millisecond apart derive: it loses the key, and keeps no input behind.
	run(t, "an opening that loses its key keeps no input", func(t *testing.T, s Store) {
		ctx := context.Background()
		var first, second string
		if _, err := opener(t, s).AppendFirstWithInput(ctx, "run-1", fresh("a"), opening("run-1", "hook:1", &first)); err != nil {
			t.Fatalf("first: %v", err)
		}
		_, err := opener(t, s).AppendFirstWithInput(ctx, "run-2", fresh("b"), opening("run-2", "hook:1", &second))
		if !errors.Is(err, ledger.ErrIdemConflict) {
			t.Fatalf("second = %v, want %v", err, ledger.ErrIdemConflict)
		}
		if _, err := opener(t, s).KeptContent(ctx, second); !errors.Is(err, domain.ErrContentAbsent) {
			t.Errorf("the losing opening's input: %v, want absent", err)
		}
		if steps, _ := s.Read(ctx, "run-2", domain.FirstSeq); len(steps) != 0 {
			t.Errorf("the losing run has %d steps, want none", len(steps))
		}
	})

	run(t, "an input past the limit keeps its prefix under the digest of all of it", func(t *testing.T, s Store) {
		ctx := context.Background()
		input := append(fresh("big"), bytes.Repeat([]byte("x"), domain.DefaultContentLimit)...)
		var named, digest string
		_, err := opener(t, s).AppendFirstWithInput(ctx, "run-1", input, func(ref, d string) domain.Step {
			digest = d
			return opening("run-1", "hook:1", &named)(ref, d)
		})
		if err != nil {
			t.Fatalf("AppendFirstWithInput: %v", err)
		}
		sum := sha256.Sum256(input)
		whole := hex.EncodeToString(sum[:])
		if digest != whole || !strings.HasSuffix(named, whole[:16]) {
			t.Errorf("named %s under %s, want the digest of the whole input %s", named, digest, whole)
		}
		// As every input is kept: a prefix and the notice that it is one, within the limit.
		want, _ := domain.Truncate(input, domain.DefaultContentLimit)
		kept, err := opener(t, s).KeptContent(ctx, named)
		if err != nil || !bytes.Equal(kept, want) {
			t.Errorf("kept %d bytes (%v), want the %d an input past the limit keeps", len(kept), err, len(want))
		}
	})

	run(t, "a step that is not this run's opening is refused", func(t *testing.T, s Store) {
		ctx := context.Background()
		var named string
		for name, build := range map[string]func(ref, digest string) domain.Step{
			"another run's":  opening("run-2", "hook:1", &named),
			"not an opening": func(string, string) domain.Step { return step("run-1", domain.StepPlanned) },
		} {
			if _, err := opener(t, s).AppendFirstWithInput(ctx, "run-1", fresh(name), build); err == nil {
				t.Errorf("%s step was accepted", name)
			}
		}
	})
}

// An erasure leaves a tombstone, and the same bytes at the same place never bring an input
// back: the run is refused with it.
func TestAppendFirstWithInput_overATombstone_opensNothing(t *testing.T) {
	pool := testdb.Isolated(t)
	ctx := context.Background()
	store := ledger.NewPostgres(pool)
	input := fresh("erased")
	ref, err := ledger.NewContent(pool).Put(ctx, "run-1", domain.FirstSeq, input)
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
	var named string
	_, err = store.AppendFirstWithInput(ctx, "run-1", input, opening("run-1", "hook:1", &named))
	if !errors.Is(err, domain.ErrContentErased) {
		t.Fatalf("err = %v, want ErrContentErased", err)
	}
	if steps, _ := store.Read(ctx, "run-1", domain.FirstSeq); len(steps) != 0 {
		t.Errorf("a run opened naming an erased input: %d steps", len(steps))
	}
}

// The rule is the database's, so it holds for a binary from before it too: two openings
// under one key at two moments, appended the way a previous release appends them, open one
// run, and the second is the idempotency conflict every release already answers with the
// run the key names.
func TestAppend_twoOpeningsUnderOneKeyAtTwoMoments_openOneRun(t *testing.T) {
	run(t, "the second opening is refused", func(t *testing.T, s Store) {
		ctx := context.Background()
		first := step("run-1", domain.StepRunStarted)
		first.IdemKey = "hook:delivery-1"
		second := step("run-2", domain.StepRunStarted)
		second.IdemKey, second.At = "hook:delivery-1", first.At.Add(time.Millisecond)
		if _, err := s.Append(ctx, first); err != nil {
			t.Fatalf("first: %v", err)
		}
		if _, err := s.Append(ctx, second); !errors.Is(err, ledger.ErrIdemConflict) {
			t.Fatalf("second = %v, want %v", err, ledger.ErrIdemConflict)
		}
		if got, err := s.RunByIdemKey(ctx, "hook:delivery-1"); err != nil || got != "run-1" {
			t.Errorf("the key names %s (%v), want the run that opened first", got, err)
		}
	})
}

type refusingSink struct{}

func (refusingSink) Put(context.Context, domain.RunID, int64, []byte) (string, error) {
	return "", errors.New("the content store is away")
}

// Both or neither, in memory as in Postgres: an input the store it is handed to cannot keep
// opens no run, and leaves nothing kept.
func TestMemory_AppendFirstWithInput_anInputThatCannotBeKept_opensNoRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := ledger.NewMemory().WithInputs(refusingSink{})
	var named string
	if _, err := m.AppendFirstWithInput(ctx, "run-1", fresh("lost"), opening("run-1", "hook:1", &named)); err == nil {
		t.Fatal("the opening succeeded with its input unkept")
	}
	if steps, _ := m.Read(ctx, "run-1", domain.FirstSeq); len(steps) != 0 {
		t.Errorf("a run opened with %d steps", len(steps))
	}
	if _, err := m.KeptContent(ctx, named); !errors.Is(err, domain.ErrContentAbsent) {
		t.Errorf("the input: %v, want absent", err)
	}
	if _, err := m.RunByIdemKey(ctx, "hook:1"); err == nil {
		t.Error("the key names a run that never opened")
	}
}
