// Package audit reads what happened, across both records that keep it.
//
// The platform keeps two append-only trails and they are not the same kind of
// thing. The run ledger is hash-chained: every step seals the one before it, so
// a step that was altered can be detected. The administrative trail records
// what people changed about the rules agents run under, and it is append-only
// by convention and by grant — nobody holds UPDATE — but it is not chained.
//
// A screen that merged them and called the result "verified" would be claiming
// a guarantee for half its rows that only the other half has. So an entry says
// which record it came from, and carries a seal only where one exists.
package audit

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Source is which record an entry came from, and therefore what can be proved
// about it.
type Source string

const (
	// SourceLedger is the run ledger: hash-chained, verifiable.
	SourceLedger Source = "ledger"
	// SourceAdmin is the administrative trail: append-only, not chained.
	SourceAdmin Source = "admin"
)

// Entry is one thing that happened, from either record.
type Entry struct {
	At     time.Time
	Source Source

	// Actor is who or what acted. A person's identifier for an administrative
	// change or a human decision; the agent for a Gate decision, because the
	// Gate decides about an agent's proposal rather than on anybody's behalf.
	Actor string
	// Verb is what happened, in the past tense and namespaced by what it
	// touched: tool.classified, gate.blocked, approval.granted.
	Verb string
	// Target is what it happened to — a tool, a run, a provider.
	Target string

	Scope  domain.Scope
	Detail map[string]any

	// RunID and Seq locate a ledger entry. Empty for administrative ones.
	RunID domain.RunID
	Seq   int64
	// Hash seals a ledger entry to the one before it. Empty for
	// administrative ones, which are append-only but not chained.
	Hash string
}

// Filter narrows the trail.
type Filter struct {
	// Scopes is what the caller may see. Empty means everything, which only
	// happens for a caller granted across the installation.
	Scopes []domain.Scope
	Since  time.Time
	Until  time.Time
	// Actor narrows to one person or agent.
	Actor string
	// Sources narrows to one record. Empty means both.
	Sources []Source
}

// Reader is the trail, declared here by the consumer.
type Reader interface {
	Read(ctx context.Context, filter Filter, limit int) ([]Entry, error)
}
