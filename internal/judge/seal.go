package judge

import (
	"encoding/binary"
	"errors"
)

/*
The sealed objects a judge run stores about its verdict (NT-016).

Each starts with the run's salt and has one fixed size, so after an erasure neither the
digest the chain keeps nor the length the tombstone keeps can be matched against the few
values a rubric declares.
*/
const (
	// SaltSize is the salt every verdict object begins with: 256 random bits.
	SaltSize = 32
	// JudgmentSize is the sealed judgment: salt, code, pass, then zeros.
	JudgmentSize = 64
	// AnswerSize is the sealed answer: salt, length, the answer, then zeros. Below the
	// content limit, so it is never truncated.
	AnswerSize = 8 << 10
)

var errJudgmentSize = errors.New("judge: a sealed judgment has one size")

// SealJudgment is the judgment object: the salt, the option's code and whether it passes.
func SealJudgment(salt []byte, o Option) []byte {
	out := make([]byte, JudgmentSize)
	copy(out, salt[:SaltSize])
	out[SaltSize] = o.Code
	if o.Passes {
		out[SaltSize+1] = 1
	}
	return out
}

// OpenJudgment reads a sealed judgment back, for a reader that still holds its bytes.
func OpenJudgment(b []byte) (code byte, passes bool, err error) {
	if len(b) != JudgmentSize {
		return 0, false, errJudgmentSize
	}
	return b[SaltSize], b[SaltSize+1] == 1, nil
}

// SealAnswer is the judge's answer as written, salted and padded, and false when it does
// not fit: an oversized answer is a failed judgment, never a truncated one.
func SealAnswer(salt []byte, answer string) ([]byte, bool) {
	if SaltSize+4+len(answer) > AnswerSize {
		return nil, false
	}
	out := make([]byte, AnswerSize)
	copy(out, salt[:SaltSize])
	binary.BigEndian.PutUint32(out[SaltSize:], uint32(len(answer)))
	copy(out[SaltSize+4:], answer)
	return out, true
}

// OpenAnswer reads a sealed answer back.
func OpenAnswer(b []byte) (string, error) {
	if len(b) != AnswerSize {
		return "", errors.New("judge: a sealed answer has one size")
	}
	n := binary.BigEndian.Uint32(b[SaltSize:])
	if int(n) > AnswerSize-SaltSize-4 {
		return "", errors.New("judge: a sealed answer's length is out of range")
	}
	return string(b[SaltSize+4 : SaltSize+4+int(n)]), nil
}
