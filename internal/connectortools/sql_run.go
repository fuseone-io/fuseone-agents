package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

var (
	// ErrNoSuchTemplate means the caller named a template this instance does
	// not register. It is the only way a query can be chosen, so this is what
	// an attempt at arbitrary SQL looks like from here.
	ErrNoSuchTemplate = errors.New("connector: no such registered template")
	// ErrParameterMismatch means the database disagrees with the template
	// about how many parameters the statement takes. The database is right.
	ErrParameterMismatch = errors.New("connector: the database reads a different number of parameters")
	// ErrResultTooLarge means the rows passed the template's bound. The rows
	// read so far are returned and the answer is marked truncated: a shorter
	// answer somebody can see beats an error that hides what was already paid
	// for.
	ErrResultTooLarge = errors.New("connector: the result passed the template's limit")
	/*
		ErrAnswerShapeTooLarge means the answer could not fit even with no rows.

		A separate sentinel because the outcomes are opposite. Truncation is a
		success with less in it; this is a failure with nothing storable, and
		collapsing them returned a payload already past the limit as though the
		limit had been kept.
	*/
	ErrAnswerShapeTooLarge = errors.New("connector: the answer's shape exceeds the template's byte limit")
	// ErrSinkOutOfOrder means an executor produced rows without columns, or
	// announced columns twice. The containment rule cannot depend on an
	// implementation remembering the order it was asked for.
	ErrSinkOutOfOrder = errors.New("connector: the executor produced rows out of order")
)

/*
SQLSession is one connection, opened for one operation.

Small on purpose: what the runtime needs is to ask the database how many
parameters a statement takes, run it read-only, and close. Everything pgx
offers beyond that would be a way for this package to grow a second opinion
about transactions.
*/
type SQLSession interface {
	// Describe asks the database what the statement takes. Authoritative: the
	// configuration check cannot read SQL, so `$1` inside a literal counts
	// there and not here.
	Describe(ctx context.Context, sql string) (int, error)
	/*
		Query runs inside a read-only transaction and reports the columns
		before the first row.

		The order is what makes an exact limit possible: the envelope of the
		stored answer depends on the column names, so a runtime told them last
		can only estimate while it decides what fits. A sink rather than two
		callbacks, so the two halves cannot be wired up separately.

		A row is json.RawMessage rather than []byte because the difference is
		not cosmetic: a [][]byte serialises as base64, so the governed answer an
		auditor reads would be an encoding of an encoding.

		The sink stops the read by returning an error, so a large answer is cut
		where it is produced rather than after it is held.
	*/
	Query(ctx context.Context, sql string, args []any, sink SQLSink) error
	/*
		Close releases the connection, and must release the transport even when
		the context expires.

		A graceful close that runs out of time has to be followed by an abrupt
		one: revocation happens after this returns, and a lease given back while
		the connection is still alive is a connection the database may keep
		serving. Returning early without releasing the socket would satisfy the
		signature and break the guarantee.
	*/
	Close(ctx context.Context) error
}

// SQLSink receives an answer as it is produced. Columns arrive once, before
// any row.
type SQLSink interface {
	Columns(names []string) error
	Row(row json.RawMessage) error
}

type SQLExecutor interface {
	Open(ctx context.Context, cfg SQLConfig, credential Credential) (SQLSession, error)
}

// revocationGrace is how long giving a lease back may take after the work is
// done. Short, and its own budget: the run's context may already be cancelled,
// and that is exactly when a lease most needs handing back.
const revocationGrace = 5 * time.Second

// closeGrace bounds the hand-back of the connection. pgx documents that Close
// can block, and an unbounded close here would swallow the revocation that
// runs after it.
const closeGrace = 3 * time.Second

/*
minTemplateBytes is the smallest byte limit worth registering.

The stored answer costs something before its first row — the object, the column
names, the provenance — so a limit under this could never hold one. Refusing it
at configuration time is better than accepting a number that guarantees an
empty answer.
*/
const minTemplateBytes = 1024

// ttlSafetyMargin keeps a query from running to the moment its credential
// expires. A statement cut off mid-result by an expiring lease looks like a
// database failure and is a clock.
const ttlSafetyMargin = 5 * time.Second

/*
RevocationOutcome is what happened to the lease, without what Vault said.

The issue asks for the revocation outcome to be recorded. Discarding the error
made that impossible and made a failed hand-back indistinguishable from a
successful one, which is the difference between a lease that expires in five
minutes and one nobody knows is open.
*/
type RevocationOutcome string

const (
	RevocationNotAttempted RevocationOutcome = "not_attempted"
	RevocationSucceeded    RevocationOutcome = "succeeded"
	RevocationFailed       RevocationOutcome = "failed"
)

/*
SQLResult is the governed answer: named columns and rows as JSON arrays.

Encoded is what the content store holds, and it is what MaxBytes bounds —
counting only the row bytes would let the envelope, the column names and the
separators carry the payload past a limit that reported itself as respected.
*/
type SQLResult struct {
	Columns    []string          `json:"columns"`
	Rows       []json.RawMessage `json:"rows"`
	Truncated  bool              `json:"truncated"`
	Issuance   Issuance          `json:"issuance"`
	Revocation RevocationOutcome `json:"revocation"`
}

type SQLRuntime struct {
	resolver *CredentialResolver
	executor SQLExecutor
}

func NewSQLRuntime(resolver *CredentialResolver, executor SQLExecutor) *SQLRuntime {
	return &SQLRuntime{resolver: resolver, executor: executor}
}

/*
Run is the whole cycle, and it is indivisible.

Authority is resolved, a connection is opened with it, the statement is checked
against the database, the read is bounded as it happens, the connection is
closed, and the lease is handed back. The order is the design: nothing reaches
the database before Vault answers, and nothing survives the call — a failure
anywhere past the credential still closes and still revokes, because both are
deferred rather than written at each exit.
*/
func (r *SQLRuntime) Run(
	ctx context.Context, instance, templateID string,
	scope domain.Scope, params map[string]any,
) (result SQLResult, err error) {
	// Not attempted until something attempts it, so a failure before issuance
	// reports a value the enum declares rather than an empty string.
	result = SQLResult{Revocation: RevocationNotAttempted}
	authority, err := r.resolver.Resolve(ctx, instance, scope)
	if err != nil {
		return result, err
	}
	// Provenance from the moment there is any. Filling it only after a
	// successful read meant a run that failed at Open or Describe recorded a
	// credential it had certainly been issued as if it never had one.
	result.Issuance = authority.Issuance()
	target := authority.Target()
	tpl, ok := target.Template(templateID)
	if !ok {
		// Resolved first and refused second, so the lease that was minted for
		// an unusable request is handed back rather than waiting out its TTL.
		result.Revocation = revoke(ctx, authority)
		return result, fmt.Errorf("%w: %s", ErrNoSuchTemplate, templateID)
	}
	args, err := bindParameters(tpl, params)
	if err != nil {
		result.Revocation = revoke(ctx, authority)
		return result, err
	}

	budget := effectiveTimeout(ctx, tpl, authority.Issuance())
	if budget <= 0 {
		result.Revocation = revoke(ctx, authority)
		return result, ErrLeaseTooShort
	}
	queryCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	session, err := r.executor.Open(queryCtx, target, authority.Credential())
	if err != nil {
		result.Revocation = revoke(ctx, authority)
		return result, reached(err, instance)
	}
	/*
		Close before revoke, always, and each on a budget of its own.

		A lease given back while a connection still holds it is a connection the
		database may keep serving. Closing can block — pgx says so — and an
		unbounded close would swallow the revocation that follows it, which is
		the failure this ordering exists to prevent. Both run on contexts
		derived without the run's cancellation, because the moment either most
		matters is the moment the run was cancelled.
	*/
	// A named return value, because what a run records has to be what happened
	// and not what was about to: assigning through a local would report an
	// outcome decided before the hand-back it describes.
	defer func() {
		closeCtx, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), closeGrace)
		_ = session.Close(closeCtx)
		cancelClose()
		result.Revocation = revoke(ctx, authority)
	}()

	if err := describes(queryCtx, session, tpl); err != nil {
		return result, err
	}
	// Into the result that already carries provenance, rather than over it:
	// assigning a fresh value here is how Issuance was lost on every path that
	// did not reach this line.
	err = read(queryCtx, session, tpl, args, &result)
	return result, err
}

// reached keeps a cancellation recognisable. A run somebody stopped and a
// database that refused are different facts, and only the second is a reason
// to look at the database.
func reached(err error, instance string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("connector: sql %s could not be reached", instance)
}

// describes asks the database and believes it. The configuration check counts
// `$n` with a regular expression and cannot see a literal or a comment, so a
// template that passed there can still bind nothing here.
func describes(ctx context.Context, session SQLSession, tpl SQLTemplate) error {
	count, err := session.Describe(ctx, tpl.SQL)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("connector: template %s could not be prepared", tpl.ID)
	}
	if count != len(tpl.Parameters) {
		return fmt.Errorf("%w: template %s declares %d and the database reads %d",
			ErrParameterMismatch, tpl.ID, len(tpl.Parameters), count)
	}
	return nil
}

/*
read stops at the template's bounds while the rows are arriving.

Counting afterwards would mean holding an answer the platform is about to
discard, and the row that crosses the byte limit is dropped rather than kept:
half a row is not a smaller answer, it is a malformed one.
*/
func read(
	ctx context.Context, session SQLSession, tpl SQLTemplate, args []any, out *SQLResult,
) error {
	sink := &boundedSink{result: out, limit: tpl.MaxBytes, maxRows: tpl.MaxRows}
	err := session.Query(ctx, tpl.SQL, args, sink)
	// Only the row bound is a success. The shape not fitting, and an executor
	// producing rows before columns, both leave nothing that may be stored.
	if errors.Is(err, ErrResultTooLarge) && !errors.Is(err, ErrAnswerShapeTooLarge) {
		return nil
	}
	if errors.Is(err, ErrAnswerShapeTooLarge) || errors.Is(err, ErrSinkOutOfOrder) {
		return err
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		// The driver's own words can quote the query, the parameters and
		// sometimes a value from a row. The template is named; nothing else is.
		return fmt.Errorf("connector: template %s failed against the database", tpl.ID)
	}
	// An executor that returned without announcing anything left a result with
	// no columns and an uncounted envelope. Silence is not an empty answer.
	if !sink.sawColumns {
		return fmt.Errorf("%w: no columns were announced", ErrSinkOutOfOrder)
	}
	return nil
}

/*
boundedSink keeps the stored answer inside the template's byte limit, exactly.

The envelope is measured rather than estimated: once the columns are known,
the result is serialised with no rows and with the longest revocation outcome
it could carry, and that is what the first row is added to. Every row after it
costs its own bytes plus the separator. An estimate would be wrong in one of
two directions, and the wrong one — under-counting — is the direction that
puts a payload past the content store while reporting the limit kept.
*/
type boundedSink struct {
	result     *SQLResult
	limit      int
	maxRows    int
	size       int
	sawColumns bool
}

func (s *boundedSink) Columns(names []string) error {
	if s.sawColumns {
		return fmt.Errorf("%w: columns were announced twice", ErrSinkOutOfOrder)
	}
	s.sawColumns = true
	s.result.Columns = names
	// The worst case of the fields that are not rows, so nothing measured here
	// can grow after the decision to stop was taken.
	// Truncated false, because false is one byte longer than true and the
	// probe has to be the largest the envelope can be. Turning it true later
	// only makes the stored answer smaller.
	probe := SQLResult{
		Columns: names, Rows: []json.RawMessage{}, Truncated: false,
		Issuance: s.result.Issuance, Revocation: RevocationNotAttempted,
	}
	encoded, err := json.Marshal(probe)
	if err != nil {
		return fmt.Errorf("connector: the answer cannot be measured")
	}
	s.size = len(encoded)
	// The shape of the answer does not fit its own limit. Truncating cannot
	// help — there is nothing left to drop — and returning a payload already
	// past the bound would be the limit reporting itself kept while the
	// content store truncates behind it. A wide result under a small template
	// limit is a configuration to correct, and columns are not known until the
	// query runs, so this is the first moment it can be said.
	if s.size > s.limit {
		s.result.Truncated = true
		return ErrAnswerShapeTooLarge
	}
	return nil
}

func (s *boundedSink) Row(row json.RawMessage) error {
	if !s.sawColumns {
		return fmt.Errorf("%w: a row arrived before the columns", ErrSinkOutOfOrder)
	}
	// The first row replaces `[]` with `[row]` and costs its own bytes; every
	// row after it also pays for the comma. Charging the separator to the
	// first row over-counted by one, which cancelled against the probe's own
	// one-byte error and stopped cancelling as soon as there were no rows.
	cost := len(row)
	if len(s.result.Rows) > 0 {
		cost++
	}
	if len(s.result.Rows) >= s.maxRows || s.size+cost > s.limit {
		s.result.Truncated = true
		return ErrResultTooLarge
	}
	s.result.Rows = append(s.result.Rows, row)
	s.size += cost
	return nil
}

/*
effectiveTimeout is the earliest of three deadlines.

The template's own bound, whatever the run has left, and the credential's TTL
less a margin. The last one is the one nobody thinks of: a statement cut off by
an expiring lease looks like a database failure and is a clock, and the margin
is what keeps the cut on this side of it.
*/
func effectiveTimeout(ctx context.Context, tpl SQLTemplate, issued Issuance) time.Duration {
	shortest := time.Duration(tpl.TimeoutSeconds) * time.Second
	if lease := time.Duration(issued.LeaseTTLSeconds)*time.Second - ttlSafetyMargin; lease < shortest {
		shortest = lease
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < shortest {
			shortest = remaining
		}
	}
	if shortest <= 0 {
		return 0
	}
	return shortest
}

// ErrLeaseTooShort means the credential expires before a query could usefully
// run. Refused before opening a connection: a context that is already spent
// makes the database look unreachable when the truth is a clock.
var ErrLeaseTooShort = errors.New("connector: the credential expires too soon to run this template")

/*
revoke gives the lease back on a budget of its own.

Derived from the run's context with the cancellation removed, because the
moment a lease most needs handing back is the moment the run was cancelled —
using the same cancelled context would skip exactly the revocation it exists
for. The short TTL remains the backstop when this fails.
*/
func revoke(ctx context.Context, authority Authority) RevocationOutcome {
	// Derived from the run's own context with the cancellation removed, rather
	// than started from Background: a fresh context drops the trace and the
	// logger, so the one call an operator most wants to find in a cancelled
	// run would be the one with no context around it.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), revocationGrace)
	defer cancel()
	if err := authority.Revoke(ctx); err != nil {
		// The outcome and not the message: a Vault error body can quote the
		// lease, the path and the policy, and this value is written into a
		// record and counted in a metric.
		return RevocationFailed
	}
	return RevocationSucceeded
}
