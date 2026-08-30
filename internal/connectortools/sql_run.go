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
	// ErrResultTooLarge means the answer passed the template's own bound. The
	// rows read so far are returned with it: a truncated answer somebody can
	// see beats an error that hides what was already paid for.
	ErrResultTooLarge = errors.New("connector: the result passed the template's limit")
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
		Query runs inside a read-only transaction and hands each row over as a
		JSON array, in the order the columns are returned.

		A row is json.RawMessage rather than [][]byte because the difference is
		not cosmetic: a [][]byte serialises as base64, so the governed result an
		auditor reads would be an encoding of an encoding. The shape is settled
		here, before pgx implements it, so the driver has one form to produce
		rather than an opinion to have.

		The callback stops the read by returning an error, so a large answer is
		cut where it is produced rather than after it is held.
	*/
	Query(ctx context.Context, sql string, args []any, row func(json.RawMessage) error) ([]string, error)
	Close(ctx context.Context) error
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

// envelopeBytes is what the stored answer costs before any row: the object,
// the column list and the fields around it. Approximate and deliberately
// generous, because being a little strict about a limit is a smaller error
// than reporting one that was not kept.
const envelopeBytes = 256

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
	authority, err := r.resolver.Resolve(ctx, instance, scope)
	if err != nil {
		return SQLResult{}, err
	}
	target := authority.Target()
	tpl, ok := target.Template(templateID)
	if !ok {
		// Resolved first and refused second, so the lease that was minted for
		// an unusable request is handed back rather than waiting out its TTL.
		outcome := revoke(authority)
		return SQLResult{Revocation: outcome}, fmt.Errorf("%w: %s", ErrNoSuchTemplate, templateID)
	}
	args, err := bindParameters(tpl, params)
	if err != nil {
		outcome := revoke(authority)
		return SQLResult{Revocation: outcome}, err
	}

	budget := effectiveTimeout(ctx, tpl, authority.Issuance())
	if budget <= 0 {
		outcome := revoke(authority)
		return SQLResult{Revocation: outcome}, ErrLeaseTooShort
	}
	queryCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	session, err := r.executor.Open(queryCtx, target, authority.Credential())
	if err != nil {
		outcome := revoke(authority)
		return SQLResult{Revocation: outcome}, reached(err, instance)
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
		result.Revocation = revoke(authority)
	}()

	if err := describes(queryCtx, session, tpl); err != nil {
		return SQLResult{}, err
	}
	result, err = read(queryCtx, session, tpl, args)
	result.Issuance = authority.Issuance()
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
	ctx context.Context, session SQLSession, tpl SQLTemplate, args []any,
) (SQLResult, error) {
	var out SQLResult
	// The envelope is paid for before the first row, because it is stored too.
	// A limit that counted only rows would report itself respected while the
	// column names and separators carried the payload past it.
	size := envelopeBytes
	columns, err := session.Query(ctx, tpl.SQL, args, func(row json.RawMessage) error {
		if len(out.Rows) >= tpl.MaxRows || size+len(row)+1 > tpl.MaxBytes {
			out.Truncated = true
			return ErrResultTooLarge
		}
		out.Rows = append(out.Rows, row)
		size += len(row) + 1
		return nil
	})
	out.Columns = columns
	if errors.Is(err, ErrResultTooLarge) {
		return out, nil
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return out, err
		}
		// The driver's own words can quote the query, the parameters and
		// sometimes a value from a row. The template is named; nothing else is.
		return out, fmt.Errorf("connector: template %s failed against the database", tpl.ID)
	}
	return out, nil
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
func revoke(authority Authority) RevocationOutcome {
	ctx, cancel := context.WithTimeout(context.Background(), revocationGrace)
	defer cancel()
	if err := authority.Revoke(ctx); err != nil {
		// The outcome and not the message: a Vault error body can quote the
		// lease, the path and the policy, and this value is written into a
		// record and counted in a metric.
		return RevocationFailed
	}
	return RevocationSucceeded
}
