package connectortools

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

const (
	EvidenceComplete = "complete"
	EvidencePartial  = "partial"
	EvidenceFloor    = "floor"

	EvidenceReasonStalled     = "paging_stalled"
	EvidenceReasonCeiling     = "event_ceiling"
	EvidenceReasonOversized   = "event_too_large"
	EvidenceReasonPageBudget  = "page_budget"
	EvidenceReasonTimeBudget  = "time_budget"
	EvidenceReasonInterrupted = "source_interrupted"

	evidenceStateKind = "evidence.state/v1"
	// evidenceBlockBytes stays under the store's payload limit, which truncates
	// without refusing. The read-back below is what proves it did not.
	evidenceBlockBytes = 900 << 10
)

// ErrEvidenceContractChanged refuses a call approved against another declaration.
var ErrEvidenceContractChanged = errors.New(
	"connector: the evidence query changed after approval")

// evidenceControl is the source's positive control over a collection's window.
type evidenceControl struct {
	Query  string `json:"query"`
	Passed bool   `json:"passed"`
}

// evidenceState is everything a resume or an analysis needs. It lives in the
// content store, so it shares the retention and erasure of the events it lists.
type evidenceState struct {
	Instance     string            `json:"instance"`
	Query        string            `json:"query"`
	Contract     string            `json:"contract"`
	Params       map[string]string `json:"params,omitempty"`
	Window       evidenceSpan      `json:"window"`
	Control      evidenceControl   `json:"control"`
	Coverage     string            `json:"coverage"`
	Reason       string            `json:"reason,omitempty"`
	Blocks       []string          `json:"blocks,omitempty"`
	Events       int64             `json:"events"`
	Unmeasured   int64             `json:"unmeasured"`
	Pages        int               `json:"pages"`
	Offset       int               `json:"offset,omitempty"`
	Cursor       string            `json:"cursor,omitempty"`
	LastPage     string            `json:"lastPage,omitempty"`
	ObservedFrom int64             `json:"observedFrom,omitempty"`
	ObservedTo   int64             `json:"observedTo,omitempty"`
}

type sealedEvidence struct {
	Kind  string          `json:"kind"`
	State json.RawMessage `json:"state"`
	Seal  string          `json:"seal"`
}

/*
evidenceSeal authenticates a state with a key only this instance holds.

A reference alone proves nothing about who wrote the content: another tool's
result in the same run could be shaped exactly like this and claim a complete
collection. Keyed by the token, so rotating it invalidates evidence in flight,
which fails closed and is one call to redo.
*/
func evidenceSeal(token string, state []byte) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(evidenceStateKind + "\x00"))
	mac.Write(state)
	return hex.EncodeToString(mac.Sum(nil))
}

// storeEvidenceBytes puts one payload and proves the store kept all of it.
func (l *Layer) storeEvidenceBytes(ctx context.Context, call engine.Call, data []byte) (string, error) {
	ref, err := l.content.Put(ctx, call.RunID, call.Seq, data)
	if err != nil {
		return "", fmt.Errorf("connector: store evidence: %w", err)
	}
	kept, err := l.content.Get(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("connector: read evidence back: %w", err)
	}
	if !bytes.Equal(kept, data) {
		return "", errors.New("connector: the content store truncated evidence")
	}
	return ref, nil
}

func (l *Layer) sealEvidence(
	ctx context.Context, call engine.Call, instance Instance, state evidenceState,
) (string, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("connector: encode evidence: %w", err)
	}
	sealed, err := json.Marshal(sealedEvidence{
		Kind: evidenceStateKind, State: raw, Seal: evidenceSeal(instance.Token, raw),
	})
	if err != nil {
		return "", fmt.Errorf("connector: encode evidence: %w", err)
	}
	return l.storeEvidenceBytes(ctx, call, sealed)
}

// openEvidence returns a state only if this run collected it through this
// instance, answering with an error code when it did not.
func (l *Layer) openEvidence(
	ctx context.Context, call engine.Call, instance Instance, ref string,
) (evidenceState, string) {
	if !strings.HasPrefix(ref, fmt.Sprintf("run://%s/", call.RunID)) {
		return evidenceState{}, CodeConnectorGuardRefused
	}
	raw, err := l.content.Get(ctx, ref)
	if err != nil {
		return evidenceState{}, CodeConnectorArtifactMissing
	}
	var sealed sealedEvidence
	if json.Unmarshal(raw, &sealed) != nil || sealed.Kind != evidenceStateKind ||
		!hmac.Equal([]byte(sealed.Seal), []byte(evidenceSeal(instance.Token, sealed.State))) {
		return evidenceState{}, CodeConnectorGuardRefused
	}
	var state evidenceState
	if json.Unmarshal(sealed.State, &state) != nil || state.Instance != instance.Name {
		return evidenceState{}, CodeConnectorGuardRefused
	}
	return state, ""
}

// evidenceBlockWriter groups projected events into blocks the store keeps whole.
type evidenceBlockWriter struct {
	l      *Layer
	call   engine.Call
	buffer bytes.Buffer
	refs   []string
}

func (w *evidenceBlockWriter) add(ctx context.Context, event []byte) error {
	if w.buffer.Len()+len(event)+1 > evidenceBlockBytes {
		if err := w.flush(ctx); err != nil {
			return err
		}
	}
	w.buffer.Write(event)
	w.buffer.WriteByte('\n')
	return nil
}

func (w *evidenceBlockWriter) flush(ctx context.Context) error {
	if w.buffer.Len() == 0 {
		return nil
	}
	ref, err := w.l.storeEvidenceBytes(ctx, w.call, w.buffer.Bytes())
	if err != nil {
		return err
	}
	w.refs = append(w.refs, ref)
	w.buffer.Reset()
	return nil
}

// evidenceAnswer is what a model reads: the base before the conclusion, and
// never an event.
func evidenceAnswer(call engine.Call, state evidenceState, ref string) map[string]any {
	answer := map[string]any{
		"operation": "evidence.collect", "instance": state.Instance, "query": state.Query,
		"scope": call.Scope.String(), "params": state.Params, "window": state.Window,
		"events": state.Events, "unmeasured": state.Unmeasured, "pages": state.Pages,
		"coverage": state.Coverage, "control": state.Control, "evidence": ref,
	}
	if state.Reason != "" {
		answer["reason"] = state.Reason
	}
	if state.ObservedFrom != 0 {
		answer["observedFrom"] = evidenceInstantText(state.ObservedFrom)
		answer["observedTo"] = evidenceInstantText(state.ObservedTo)
	}
	return answer
}

func evidenceInstantText(epoch int64) string {
	return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
}

func evidenceLabels() domain.Labels {
	return domain.NewLabels(domain.LabelUntrusted)
}
