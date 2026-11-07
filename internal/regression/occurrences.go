package regression

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

// Content is the claim check the occurrences live behind, declared here by
// the consumer.
type Content interface {
	Get(ctx context.Context, ref string) ([]byte, error)
}

// Corpus reads the cases of a corpus with their bytes, ready to be replayed.
//
// The store keeps a reference and never the occurrence itself: a case is real
// customer material and belongs under the installation's retention like every
// other bulky payload (AU-04). Reading one is therefore two reads, and this is
// where they are joined.
type Corpus struct {
	cases   *Store
	content Content
}

func NewCorpus(cases *Store, content Content) *Corpus {
	return &Corpus{cases: cases, content: content}
}

// Hold fences the corpus against an erasure while it is being copied.
func (c *Corpus) Hold(
	ctx context.Context, agent domain.AgentID, inside func(context.Context) error,
) error {
	return c.cases.Hold(ctx, agent, inside)
}

/*
Occurrences reads the cases of one agent's corpus that can still run.

A case whose occurrence was erased is left out, and the battery reports it as a
gap, because one aged-out case must not stop every other correction being
checked. Anything else that cannot be read refuses the whole corpus: a battery
missing a case for no stated reason reports green on a correction nobody
checked, which is the one failure a safety net must not have.
*/
func (c *Corpus) Occurrences(
	ctx context.Context, agent domain.AgentID,
) (simulate.Corpus, error) {
	version, cases, err := c.cases.Pinned(ctx, agent)
	if err != nil {
		return simulate.Corpus{}, err
	}

	out := simulate.Corpus{Version: version, Cases: make([]simulate.Occurrence, 0, len(cases))}
	for _, one := range cases {
		if one.Erased {
			continue
		}
		input, err := c.content.Get(ctx, one.InputRef)
		if errors.Is(err, domain.ErrContentErased) {
			continue
		}
		if err != nil {
			return simulate.Corpus{}, fmt.Errorf("the occurrence of %s could not be read: %w", one.ID, err)
		}
		out.Cases = append(out.Cases, simulate.Occurrence{ID: one.ID, Input: input})
	}
	return out, nil
}
