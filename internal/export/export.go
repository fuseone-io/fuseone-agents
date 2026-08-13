/*
Package export produces a range of the ledger somebody outside this
installation can check without trusting it (PRD AU-12).

Two mechanisms, and both have to hold for an export to mean anything.

The hash chain proves the content: every step's hash commits to all of its
fields and to the previous step's hash, so editing a payload, reordering two
steps or removing an inconvenient one from the middle all break a link. That
much is true of the ledger itself and is what NF-05 rests on.

The signature proves the range came from here. Without it the chain is
self-consistent and anybody could produce one — it says these steps follow
each other, not that they are ours.
*/
package export

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Format names the shape, in the document, because an auditor opening this in
// five years needs to know what they are reading before they can check it.
const Format = "fuseone.ledger.v1"

// signingDomain separates this signature from every other use of the same
// key. Without it a signature over an export could be replayed as a signature
// over something else that happened to hash the same way.
const signingDomain = "fuseone.export.v1"

var (
	ErrEmptyRange   = errors.New("export: there is nothing in that range to sign")
	ErrBadSignature = errors.New("export: the signature does not match the steps")
	ErrWrongFormat  = errors.New("export: not a ledger export")
)

// Bundle is a signed range of the ledger.
type Bundle struct {
	Format     string        `json:"format"`
	Company    string        `json:"company"`
	ExportedAt time.Time     `json:"exportedAt"`
	Steps      []domain.Step `json:"-"`

	// PublicKey travels with the bundle so a reader can check the signature
	// without asking anybody for it. It proves nothing on its own — what makes
	// it worth anything is the reader comparing it against the key this
	// installation publishes.
	PublicKey ed25519.PublicKey `json:"publicKey"`
	Signature []byte            `json:"signature"`
}

// Build seals a range.
func Build(company string, steps []domain.Step, key ed25519.PrivateKey) (Bundle, error) {
	if len(steps) == 0 {
		// A signed statement that nothing happened is a statement somebody can
		// wave about. If there is nothing to export, say so.
		return Bundle{}, ErrEmptyRange
	}
	if err := domain.VerifyChain(steps); err != nil {
		// Signing a range this installation cannot itself verify would put our
		// name on a chain we know is broken.
		return Bundle{}, fmt.Errorf("export: refusing to sign a broken chain: %w", err)
	}

	bundle := Bundle{
		Format: Format, Company: company,
		ExportedAt: time.Now().UTC().Truncate(time.Second),
		Steps:      steps,
		PublicKey:  key.Public().(ed25519.PublicKey),
	}
	bundle.Signature = ed25519.Sign(key, bundle.digest())
	return bundle, nil
}

/*
digest is what the signature covers.

Built from the steps' hashes rather than from the serialised document, so the
signature survives any reformatting on the way — an export that stopped
verifying because somebody pretty-printed the JSON would be useless in the one
situation it exists for.

Each hash already commits to every field of its step, so covering the hashes
covers the content. The count is in there because a chain is a prefix of
itself: without it, truncating the export would produce a shorter chain that
still verifies.
*/
func (b Bundle) digest() []byte {
	h := sha256.New()
	h.Write([]byte(signingDomain))
	h.Write([]byte(b.Company))
	h.Write([]byte(b.ExportedAt.Format(time.RFC3339)))

	var count [8]byte
	for i, n := 0, len(b.Steps); i < 8; i++ {
		count[i] = byte(n >> (8 * i))
	}
	h.Write(count[:])

	for _, step := range b.Steps {
		h.Write(step.Hash)
	}
	return h.Sum(nil)
}

// Fingerprint identifies the key that signed this, short enough to read out
// loud when somebody checks it against what the installation publishes.
func (b Bundle) Fingerprint() string {
	sum := sha256.Sum256(b.PublicKey)
	return hex.EncodeToString(sum[:8])
}

// Verify checks the chain and then the signature.
//
// In that order deliberately: a broken chain is a statement about the content,
// and reporting a signature failure for it would send somebody looking at the
// wrong thing.
func Verify(b Bundle) error {
	if b.Format != Format {
		return fmt.Errorf("%w: %q", ErrWrongFormat, b.Format)
	}
	if len(b.Steps) == 0 {
		return ErrEmptyRange
	}
	if err := domain.VerifyChain(b.Steps); err != nil {
		return fmt.Errorf("export: the chain does not hold: %w", err)
	}
	if len(b.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: no usable public key", ErrBadSignature)
	}
	if !ed25519.Verify(b.PublicKey, b.digest(), b.Signature) {
		return ErrBadSignature
	}
	return nil
}

// document is the bundle as it appears on disk. The steps travel in the
// export's own shape rather than the domain's, so the format outlives any
// refactor behind it.
type document struct {
	Bundle
	Steps []step `json:"steps"`
}

// Encode writes the bundle as indented JSON.
//
// Indented because a person opens this, and a single line of ten thousand
// steps is a file nobody reads. The signature covers the steps' hashes rather
// than these bytes, so the formatting is free to be legible.
func (b Bundle) Encode() ([]byte, error) {
	doc := document{Bundle: b, Steps: make([]step, 0, len(b.Steps))}
	for _, s := range b.Steps {
		doc.Steps = append(doc.Steps, toStep(s))
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("export: encode: %w", err)
	}
	return out, nil
}

func Decode(raw []byte) (Bundle, error) {
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Bundle{}, fmt.Errorf("export: decode: %w", err)
	}

	out := doc.Bundle
	out.Steps = make([]domain.Step, 0, len(doc.Steps))
	for _, s := range doc.Steps {
		step, err := fromStep(s)
		if err != nil {
			return Bundle{}, err
		}
		out.Steps = append(out.Steps, step)
	}
	return out, nil
}
