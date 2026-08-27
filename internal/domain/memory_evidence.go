package domain

import (
	"strconv"
	"strings"
)

/*
MemoryEvidence is one citation: which bytes in the ledger a memory rests on.

Seq and SourceRunID are omitempty because evidence written before they existed
has neither. Seq zero means "the run, not a step of it", which is what the older
shape meant; an absent SourceRunID means the run that produced the bytes is the
run that cited them.

Those two are not the same kind of absence. An absent source is answerable from
the record itself, so SourceRun resolves it and Key never sees the difference.
An absent Seq is not: nothing in the row says which step, and the digest stored
with it is the truncated one from the reference. So a legacy citation and its
hydrated form have different keys, deliberately — hydration recognises the older
shape and replaces it, rather than pretending the two were always one. Key
identifies the complete form.

Labels are what the cited step had accumulated at that point, resolved from the
ledger and never accepted from a caller. They live on the citation rather than
only on the assertion so a merge can answer which evidence explains which label
— and refuse to evict the one that does.
*/
type MemoryEvidence struct {
	RunID RunID `json:"run_id"`
	// SourceRunID is the run that produced the bytes, when a context artifact
	// makes that a different run from the one citing them.
	SourceRunID RunID  `json:"source_run_id,omitempty"`
	Seq         int64  `json:"seq,omitempty"`
	Artifact    string `json:"artifact"`
	Digest      string `json:"digest"`
	Labels      Labels `json:"labels,omitempty"`
}

/*
Key is the identity of the citation, which is deliberately not the whole struct.

Labels are excluded: the resolver derives them from the ledger, so the same step
read at two moments can arrive with different labels — before and after the run
gained taint, before and after a reconciliation filled them in. Those are one
citation, and treating them as two would spend the eight-record budget on a
duplicate of itself.

A string rather than the struct as a map key, because Labels is a slice and a
slice makes a struct uncomparable. That is the mechanical reason; the reason
above is why it would have been wrong to keep comparing the whole thing anyway.
*/
func (e MemoryEvidence) Key() string {
	var b strings.Builder
	for _, part := range []string{
		string(e.RunID), string(e.SourceRun()),
		strconv.FormatInt(e.Seq, 10), e.Artifact, e.Digest,
	} {
		b.WriteString(part)
		b.WriteByte(0)
	}
	return b.String()
}

/*
SourceRun is the run that produced the bytes, spelled out.

Absent means the citing run, so a caller must never read SourceRunID directly:
the two spellings are the same citation, and treating them apart would rename
every legacy record the moment hydration filled the field in.
*/
func (e MemoryEvidence) SourceRun() RunID {
	if e.SourceRunID == "" {
		return e.RunID
	}
	return e.SourceRunID
}
