package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strconv"
	"time"

	"github.com/fuseone/agents/internal/engine"
)

const (
	// evidencePagesPerCall and evidenceCallBudget bound one tool call. Running
	// out is an answer, `partial`, and the next call crosses the Gate and the
	// ledger like any other: no goroutine outlives a call, so nothing needs
	// reconciling after a crash, and a run keeps its one writer.
	evidencePagesPerCall = 50
	evidenceCallBudget   = 4 * time.Minute
)

// evidenceCollection is one call's work on a state.
type evidenceCollection struct {
	l        *Layer
	call     engine.Call
	instance Instance
	query    EvidenceQuery
	filters  url.Values
	state    evidenceState
	blocks   evidenceBlockWriter
	seen     map[string]struct{}
	// taken counts this call's pages: a failure before the first has nothing to
	// keep and is the call's failure, one after it interrupts a collection.
	taken int
}

func (c *evidenceCollection) credential() SecretValue {
	return SecretValue{value: c.instance.Token}
}

// fetch asks for the page the state points at.
func (c *evidenceCollection) fetch(ctx context.Context) (evidencePage, error) {
	cfg := c.instance.Evidence
	values := maps.Clone(c.filters)
	applyEvidenceSpan(values, cfg.Window, c.state.Window)
	values.Set(cfg.Paging.LimitParam, strconv.Itoa(cfg.Paging.PageSize))
	switch {
	case cfg.Paging.Kind == EvidencePagingOffset && c.state.Offset > 0:
		values.Set(cfg.Paging.OffsetParam, strconv.Itoa(c.state.Offset))
	case cfg.Paging.Kind == EvidencePagingCursor && c.state.Cursor != "":
		values.Set(cfg.Paging.CursorParam, c.state.Cursor)
	}
	body, err := c.l.evidence.Fetch(ctx, cfg, c.credential(),
		EvidenceRequest{Path: c.query.Path, Query: values})
	if err != nil {
		return evidencePage{}, err
	}
	return parseEvidencePage(cfg, body)
}

// run pages until the source ends, a limit is reached or the budget runs out.
func (c *evidenceCollection) run(ctx context.Context) error {
	budget, cancel := context.WithTimeout(ctx, evidenceCallBudget)
	defer cancel()
	for c.taken < evidencePagesPerCall {
		page, err := c.fetch(budget)
		if err != nil {
			return c.interrupted(ctx, err)
		}
		c.taken++
		c.state.Pages++
		done, err := c.take(ctx, page)
		if err != nil || done {
			return err
		}
	}
	c.settle(EvidencePartial, EvidenceReasonPageBudget)
	return nil
}

func (c *evidenceCollection) settle(coverage, reason string) {
	c.state.Coverage, c.state.Reason = coverage, reason
}

// interrupted decides what a failed page means for the pages already held.
func (c *evidenceCollection) interrupted(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		c.settle(EvidencePartial, EvidenceReasonTimeBudget)
		return nil
	}
	if c.taken == 0 {
		return err
	}
	c.settle(EvidencePartial, EvidenceReasonInterrupted)
	return nil
}

// take stores one page and says whether the collection is finished.
func (c *evidenceCollection) take(ctx context.Context, page evidencePage) (bool, error) {
	if len(page.rows) == 0 {
		c.settle(EvidenceComplete, "")
		return true, nil
	}
	if page.digest == c.state.LastPage {
		c.settle(EvidenceFloor, EvidenceReasonStalled)
		return true, nil
	}
	c.state.LastPage = page.digest
	fresh, stop, err := c.store(ctx, page)
	if err != nil || stop {
		return stop, err
	}
	if fresh == 0 {
		c.settle(EvidenceFloor, EvidenceReasonStalled)
		return true, nil
	}
	return c.advance(page), nil
}

// store keeps a page's new events, reporting how many and whether a limit ended
// the collection.
func (c *evidenceCollection) store(ctx context.Context, page evidencePage) (int, bool, error) {
	fresh := 0
	for _, row := range page.rows {
		if c.state.Events >= evidenceCeiling(c.instance.Evidence) {
			c.settle(EvidenceFloor, EvidenceReasonCeiling)
			return fresh, true, nil
		}
		if c.duplicate(row) {
			continue
		}
		event, err := projectEvidence(row, c.query.Fields)
		if err != nil {
			return fresh, false, fmt.Errorf("connector: encode event: %w", err)
		}
		if len(event) >= evidenceBlockBytes {
			c.settle(EvidenceFloor, EvidenceReasonOversized)
			return fresh, true, nil
		}
		if err := c.blocks.add(ctx, event); err != nil {
			return fresh, false, err
		}
		c.count(row)
		fresh++
	}
	return fresh, false, nil
}

func (c *evidenceCollection) duplicate(row map[string]json.RawMessage) bool {
	if c.query.IDField == "" {
		return false
	}
	id := evidenceScalar(row[c.query.IDField])
	if id == "" {
		return false
	}
	if _, held := c.seen[id]; held {
		return true
	}
	c.seen[id] = struct{}{}
	return false
}

func (c *evidenceCollection) count(row map[string]json.RawMessage) {
	c.state.Events++
	gap := c.instance.Evidence.Gap
	if gap.Field != "" && evidenceScalar(row[gap.Field]) == gap.Value {
		c.state.Unmeasured++
	}
	if at, ok := evidenceInstantOf(row[c.query.TimeField]); ok {
		if c.state.ObservedFrom == 0 || at < c.state.ObservedFrom {
			c.state.ObservedFrom = at
		}
		if at > c.state.ObservedTo {
			c.state.ObservedTo = at
		}
	}
}

// advance moves to the next page, or finishes when the source has no more.
func (c *evidenceCollection) advance(page evidencePage) bool {
	paging := c.instance.Evidence.Paging
	if paging.Kind == EvidencePagingCursor {
		c.state.Cursor = page.next
		if page.next == "" {
			c.settle(EvidenceComplete, "")
			return true
		}
		return false
	}
	c.state.Offset += len(page.rows)
	if len(page.rows) < paging.PageSize {
		c.settle(EvidenceComplete, "")
		return true
	}
	return false
}

// runEvidenceControl asks for one event of the unfiltered query over a window.
// Credentials and refusals are the caller's failure; an empty answer is the
// control's result.
func (l *Layer) runEvidenceControl(
	ctx context.Context, instance Instance, span evidenceSpan,
) (evidenceControl, error) {
	cfg := instance.Evidence
	control, found := evidenceQueryByID(cfg, cfg.Control)
	if !found {
		return evidenceControl{}, fmt.Errorf("connector: evidence control %q is not declared", cfg.Control)
	}
	values, _ := evidenceFilters(control, nil)
	applyEvidenceSpan(values, cfg.Window, span)
	values.Set(cfg.Paging.LimitParam, "1")
	body, err := l.evidence.Fetch(ctx, cfg, SecretValue{value: instance.Token},
		EvidenceRequest{Path: control.Path, Query: values})
	if err != nil {
		return evidenceControl{}, err
	}
	page, err := parseEvidencePage(cfg, body)
	if err != nil {
		return evidenceControl{}, err
	}
	return evidenceControl{Query: control.ID, Passed: len(page.rows) > 0}, nil
}
