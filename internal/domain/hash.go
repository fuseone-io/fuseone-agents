package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"hash"
)

var (
	ErrChainBroken  = errors.New("ledger chain broken: prev hash mismatch")
	ErrHashMismatch = errors.New("ledger chain broken: step hash mismatch")
)

// hashDomain namespaces the digest so a hash computed here can never collide
// with one computed over a different structure that happens to share bytes.
const hashDomain = "fuseone.agents.step.v1"

// computeHash seals the step: SHA-256 over the previous hash followed by a
// canonical encoding of every field.
//
// Every field is length-prefixed. Without that, the concatenation of ("ab",
// "c") and ("a", "bc") would digest identically, and an attacker who controls
// two adjacent fields could forge an entry that verifies.
func (s Step) computeHash() []byte {
	h := sha256.New()
	writeBytes(h, []byte(hashDomain))
	writeBytes(h, s.PrevHash)

	writeBytes(h, []byte(s.RunID))
	writeInt(h, s.Seq)
	writeBytes(h, []byte(s.Kind))
	writeBytes(h, []byte(s.Scope.Company))
	writeBytes(h, []byte(s.Scope.Area))
	writeBytes(h, []byte(s.AgentID))
	writeBytes(h, []byte(s.VersionID))
	writeBytes(h, []byte(s.OnBehalfOf))
	writeBytes(h, s.Payload)

	// Labels are already sorted and deduplicated by NewLabels; the count guards
	// against a set being confused with a single concatenated label.
	writeInt(h, int64(len(s.Labels)))
	for _, l := range s.Labels {
		writeBytes(h, []byte(l))
	}

	writeInt(h, s.Cost.InputTokens)
	writeInt(h, s.Cost.OutputTokens)
	writeInt(h, s.Cost.CacheReadTokens)
	writeInt(h, s.Cost.CacheWriteTokens)
	writeInt(h, s.Cost.Micros)

	writeBytes(h, []byte(s.IdemKey))
	writeBytes(h, []byte(s.PolicyHash))
	writeInt(h, s.At.UTC().UnixMicro())

	return h.Sum(nil)
}

func writeBytes(h hash.Hash, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	_, _ = h.Write(n[:])
	_, _ = h.Write(b)
}

func writeInt(h hash.Hash, v int64) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(v))
	_, _ = h.Write(n[:])
}

// equalBytes compares in constant time. The ledger head is exposed over the
// API for external verification, and variable-time comparison there leaks the
// digest one byte at a time.
func equalBytes(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// VerifyChain walks steps in order and checks every link. It is the mechanism
// behind the signed export (PRD AU-12) and the tamper alarm (NF-05).
func VerifyChain(steps []Step) error {
	var prev *Step
	for i := range steps {
		if err := steps[i].VerifyLink(prev); err != nil {
			return err
		}
		prev = &steps[i]
	}
	return nil
}
