package blocks

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrExcerpt is an edit whose excerpt is not in its block exactly once: empty,
// absent or repeated, it does not say which bytes it changes.
var ErrExcerpt = errors.New("blocks: the excerpt is not in the block exactly once")

/*
Edit replaces one excerpt of a block, found exactly once, and keeps the rest
of the block as it is. It is what makes a change to a long block fit a turn:
rewriting a block of thousands of words whole is more than a model's answer
holds. The block keeps its label, since an excerpt does not retitle it; an
edit that changes nothing leaves the block's bytes as they were.
*/
func Edit(doc []Block, id, old, replacement string) ([]Block, error) {
	i := indexOf(doc, id)
	if i < 0 {
		return nil, fmt.Errorf("%w: edit %s", ErrUnknownBlock, id)
	}
	if old == "" {
		return nil, fmt.Errorf("%w: edit %s names no excerpt", ErrExcerpt, id)
	}
	if n := strings.Count(doc[i].Text, old); n != 1 {
		return nil, fmt.Errorf("%w: edit %s finds it %d times", ErrExcerpt, id, n)
	}
	out := slices.Clone(doc)
	text := strings.TrimSpace(strings.Replace(out[i].Text, old, replacement, 1))
	if text == out[i].Text {
		return out, nil
	}
	out[i] = Block{ID: id, Kind: out[i].Kind, Label: out[i].Label, Text: text}
	return out, nil
}
