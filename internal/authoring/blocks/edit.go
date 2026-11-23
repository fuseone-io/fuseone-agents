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
	at, ok := onlyPlace(doc[i].Text, old)
	if !ok {
		return nil, fmt.Errorf("%w: edit %s", ErrExcerpt, id)
	}
	out := slices.Clone(doc)
	text := strings.TrimSpace(out[i].Text[:at] + replacement + out[i].Text[at+len(old):])
	if text == out[i].Text {
		return out, nil
	}
	out[i] = Block{ID: id, Kind: out[i].Kind, Label: out[i].Label, Text: text}
	return out, nil
}

/*
onlyPlace is where an excerpt starts when it starts at exactly one place. The
second search begins one byte past the first start, not past its end, so
overlapping occurrences count: in "aaa", "aa" starts at two places, which
strings.Count, counting matches that do not overlap, reports as one. A byte
past a start is safe in UTF-8: an excerpt's first byte is never a
continuation byte, so no match begins inside a character.
*/
func onlyPlace(text, excerpt string) (int, bool) {
	at := strings.Index(text, excerpt)
	if at < 0 || strings.Contains(text[at+1:], excerpt) {
		return 0, false
	}
	return at, true
}
