package draft

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/spec"
)

// Stored is a draft as its row holds it: the definition in the publication's
// own format, with no body, and the blocks with their ids and raw bytes.
type Stored struct {
	Definition string
	Blocks     []byte
}

type storedBlock struct {
	ID    string      `json:"id"`
	Kind  blocks.Kind `json:"kind"`
	Label string      `json:"label,omitempty"`
	Text  string      `json:"text"`
	Raw   string      `json:"raw,omitempty"`
}

// Encode writes a draft for its row. The definition is spec.Render's, so the
// stored form is the format every other path reads.
func Encode(d Draft) (Stored, error) {
	s := d.Spec
	s.Instructions = ""
	definition, err := spec.Render(s)
	if err != nil {
		return Stored{}, fmt.Errorf("draft: encode the definition: %w", err)
	}
	out := make([]storedBlock, len(d.Blocks))
	for i, b := range d.Blocks {
		out[i] = storedBlock(b)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return Stored{}, fmt.Errorf("draft: encode the blocks: %w", err)
	}
	return Stored{Definition: string(definition), Blocks: encoded}, nil
}

// Decode reads a draft's row back. What the draft still lacks is not an
// error here: the store holds drafts long before they are publishable.
func Decode(st Stored) (Draft, error) {
	s, _, err := spec.ParseDraft("draft", []byte(st.Definition))
	if err != nil {
		return Draft{}, fmt.Errorf("draft: decode the definition: %w", err)
	}
	var read []storedBlock
	if err := json.Unmarshal(st.Blocks, &read); err != nil {
		return Draft{}, fmt.Errorf("draft: decode the blocks: %w", err)
	}
	d := Draft{Spec: s}
	d.Spec.Version, d.Spec.Digest, d.Spec.Source = "", "", ""
	for _, b := range read {
		d.Blocks = append(d.Blocks, blocks.Block(b))
	}
	return d, nil
}

// Body is what an operation said, as the content store keeps it: its kind
// and its payload. The trail holds only Body's digest and a reference to it.
func Body(op Operation) []byte {
	body, err := json.Marshal(struct {
		Kind      string    `json:"kind"`
		Operation Operation `json:"operation"`
	}{op.Kind(), op})
	if err != nil {
		// Every operation is plain data; one that does not encode is a
		// programming error the body must still not hide.
		return []byte(fmt.Sprintf(`{"kind":%q,"unencodable":%q}`, op.Kind(), fmt.Sprintf("%#v", op)))
	}
	return body
}

// Digest is the SHA-256 of an operation's Body, in hex: what the trail keeps
// of it, since a trail row holds no free text.
func Digest(op Operation) string {
	sum := sha256.Sum256(Body(op))
	return hex.EncodeToString(sum[:])
}
