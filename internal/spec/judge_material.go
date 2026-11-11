package spec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/judge"
)

// maxJudgeRepetitions is how many times one output may be judged, as for an experiment.
const maxJudgeRepetitions = 5

var errJudgeOpening = errors.New("the opening does not describe a judgment")

// judgeMaterial is everything a judge run's opening names, read and checked.
type judgeMaterial struct {
	rubric judge.Rubric
	salt   []byte
	input  []byte
	output []byte
}

// readJudgeMaterial reads what a judge run's opening names and refuses any object whose
// bytes are not the ones its digest recorded.
func readJudgeMaterial(
	ctx context.Context, content Content, started domain.RunStartedPayload,
) (judgeMaterial, error) {
	o := started.Judge
	if err := validJudgeOpening(o); err != nil {
		return judgeMaterial{}, err
	}
	var m judgeMaterial
	raw, err := readMatching(ctx, content, o.RubricRef, o.RubricDigest, fullDigest)
	if err != nil {
		return judgeMaterial{}, fmt.Errorf("rubric: %w", err)
	}
	if m.rubric, err = judge.ParseRubric(raw); err != nil || m.rubric.Digest() != o.RubricDigest {
		return judgeMaterial{}, fmt.Errorf("rubric: not the one recorded: %v", err)
	}
	if m.salt, err = readMatching(ctx, content, o.SaltRef, o.SaltDigest, fullDigest); err != nil {
		return judgeMaterial{}, fmt.Errorf("salt: %w", err)
	}
	if len(m.salt) != judge.SaltSize {
		return judgeMaterial{}, fmt.Errorf("salt: %d bytes, want %d", len(m.salt), judge.SaltSize)
	}
	if m.output, err = readMatching(ctx, content, o.OutputRef, o.OutputDigest, shortDigest); err != nil {
		return judgeMaterial{}, fmt.Errorf("judged output: %w", err)
	}
	if started.InputRef != "" {
		if m.input, err = content.Get(ctx, started.InputRef); err != nil {
			return judgeMaterial{}, fmt.Errorf("case input: %w", err)
		}
	}
	return m, nil
}

func validJudgeOpening(o *domain.JudgeOpening) error {
	switch {
	case o == nil:
		return errJudgeOpening
	case o.Provider == "" || o.Model == "":
		return fmt.Errorf("%w: no judge pair", errJudgeOpening)
	case o.CeilingMicros <= 0:
		return fmt.Errorf("%w: no per-run ceiling", errJudgeOpening)
	case o.Repetitions < 1 || o.Repetitions > maxJudgeRepetitions:
		return fmt.Errorf("%w: %d repetitions", errJudgeOpening, o.Repetitions)
	case o.Repetition < 1 || o.Repetition > o.Repetitions:
		return fmt.Errorf("%w: repetition %d of %d", errJudgeOpening, o.Repetition, o.Repetitions)
	}
	return nil
}

// readMatching reads a reference and refuses bytes whose digest is not the recorded one.
func readMatching(
	ctx context.Context, content Content, ref, want string, digestOf func([]byte) string,
) ([]byte, error) {
	if ref == "" || want == "" {
		return nil, errors.New("not named")
	}
	b, err := content.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	if digestOf(b) != want {
		return nil, errors.New("not the bytes recorded")
	}
	return b, nil
}

// fullDigest is a rubric's or a salt's: the whole sha256.
func fullDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// shortDigest is the one the engine seals for an outcome, which the judged run recorded.
func shortDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}
