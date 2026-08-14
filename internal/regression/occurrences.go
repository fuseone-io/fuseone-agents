package regression

import (
	"context"
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

// Occurrences reads every case of one agent's corpus.
//
// Refused whole rather than run short. A battery missing a case is a battery
// that reports green on a correction nobody checked, which is the one failure
// a safety net must not have.
func (c *Corpus) Occurrences(
	ctx context.Context, agent domain.AgentID,
) ([]simulate.Occurrence, error) {
	cases, err := c.cases.List(ctx, agent)
	if err != nil {
		return nil, err
	}

	out := make([]simulate.Occurrence, 0, len(cases))
	for _, one := range cases {
		input, err := c.content.Get(ctx, one.InputRef)
		if err != nil {
			return nil, fmt.Errorf("the occurrence of %s could not be read: %w", one.ID, err)
		}
		out = append(out, simulate.Occurrence{ID: one.ID, Input: input})
	}
	return out, nil
}
