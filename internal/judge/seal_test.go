package judge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

/*
Nothing left after erasure gives the value back (NT-016).

A rubric declares at most eleven values. What survives an erasure is a digest, a length
and a flag; any of them that differs between values reads the value back. Every verdict
object is salted and of one fixed size, and the answer is a fixed-width code.
*/

func salt(b byte) []byte { return bytes.Repeat([]byte{b}, SaltSize) }

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestSealJudgment_isOneSizeForEveryValue(t *testing.T) {
	t.Parallel()
	r := mustRubric(t, `{"instructions":"x","output":{"type":"score","min":0,"max":10,"threshold":7}}`)
	for _, o := range r.Options() {
		if got := len(SealJudgment(salt(7), o)); got != JudgmentSize {
			t.Errorf("judgment of %s is %d bytes, want %d", o.Value, got, JudgmentSize)
		}
	}
}

func TestSealJudgment_opensToItsCodeAndPass(t *testing.T) {
	t.Parallel()
	o := Option{'B', "high_risk", false}
	code, passes, err := OpenJudgment(SealJudgment(salt(1), o))
	if err != nil || code != 'B' || passes {
		t.Errorf("OpenJudgment = %c %v %v, want B, not passing", code, passes, err)
	}
	if _, _, err := OpenJudgment([]byte("short")); err == nil {
		t.Error("a judgment of the wrong size opened")
	}
}

func TestSealAnswer_isOneSizeWhateverItSays(t *testing.T) {
	t.Parallel()
	for _, answer := range []string{"A", "K", "A\nshort", "B\n" + string(bytes.Repeat([]byte("x"), 4000))} {
		sealed, ok := SealAnswer(salt(3), answer)
		if !ok || len(sealed) != AnswerSize {
			t.Errorf("answer of %d bytes sealed to %d ok=%v, want %d", len(answer), len(sealed), ok, AnswerSize)
		}
	}
}

func TestSealAnswer_oneByteOverTheSize_isOversized(t *testing.T) {
	t.Parallel()
	fits := string(bytes.Repeat([]byte("x"), AnswerSize-SaltSize-4))
	if _, ok := SealAnswer(salt(3), fits); !ok {
		t.Fatal("the largest answer that fits was refused")
	}
	if _, ok := SealAnswer(salt(3), fits+"x"); ok {
		t.Error("an answer one byte over the size was sealed, want oversized")
	}
}

// What an erased object leaves is its digest and its length. Enumerating every declared
// output, as a judgment and as a bare answer, must not reproduce either digest, and the
// lengths must not tell them apart.
func TestSealed_digestsAndLengths_doNotGiveTheValueBack(t *testing.T) {
	t.Parallel()
	r := mustRubric(t, categoryRubric)
	secret := salt(9)
	recordedJudgment := SealJudgment(secret, r.Options()[1])
	recordedAnswer, _ := SealAnswer(secret, "B")

	for _, o := range r.Options() {
		for _, guess := range [][]byte{
			SealJudgment(make([]byte, SaltSize), o),
			[]byte{o.Code}, []byte(o.Value),
		} {
			if shaHex(guess) == shaHex(recordedJudgment) || shaHex(guess) == shaHex(recordedAnswer) {
				t.Errorf("enumerating %s reproduced a recorded digest", o.Value)
			}
		}
		other := SealJudgment(secret, o)
		answer, _ := SealAnswer(secret, string(o.Code))
		if len(other) != len(recordedJudgment) || len(answer) != len(recordedAnswer) {
			t.Errorf("%s is told apart by its length", o.Value)
		}
	}
}
