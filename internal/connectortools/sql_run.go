package connectortools

import (
	"context"
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
	// Query runs inside a read-only transaction and hands each row over as it
	// arrives. The callback stops the read by returning an error, so a large
	// answer is cut where it is produced rather than after it is held.
	Query(ctx context.Context, sql string, args []any, row func([]byte) error) error
	Close(ctx context.Context) error
}

type SQLExecutor interface {
	Open(ctx context.Context, cfg SQLConfig, credential Credential) (SQLSession, error)
}

// revocationGrace is how long giving a lease back may take after the work is
// done. Short, and its own budget: the run's context may already be cancelled,
// and that is exactly when a lease most needs handing back.
const revocationGrace = 5 * time.Second

// ttlSafetyMargin keeps a query from running to the moment its credential
// expires. A statement cut off mid-result by an expiring lease looks like a
// database failure and is a clock.
const ttlSafetyMargin = 5 * time.Second

type SQLResult struct {
	Rows      [][]byte
	Truncated bool
	Issuance  Issuance
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
) (SQLResult, error) {
	authority, err := r.resolver.Resolve(ctx, instance, scope)
	if err != nil {
		return SQLResult{}, err
	}
	target := authority.Target()
	tpl, ok := target.Template(templateID)
	if !ok {
		// Resolved first and refused second, so the lease that was minted for
		// an unusable request is handed back rather than waiting out its TTL.
		revoke(authority)
		return SQLResult{}, fmt.Errorf("%w: %s", ErrNoSuchTemplate, templateID)
	}
	args, err := bindParameters(tpl, params)
	if err != nil {
		revoke(authority)
		return SQLResult{}, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, effectiveTimeout(ctx, tpl, authority.Issuance()))
	defer cancel()

	session, err := r.executor.Open(queryCtx, target, authority.Credential())
	if err != nil {
		revoke(authority)
		return SQLResult{}, fmt.Errorf("connector: sql %s could not be reached", instance)
	}
	// Close before revoke, always. A lease given back while a connection still
	// holds it is a connection the database may keep serving, and the ordering
	// is deferred so a panic takes the same path as a return.
	defer func() {
		_ = session.Close(withoutCancel(ctx))
		revoke(authority)
	}()

	if err := describes(queryCtx, session, tpl); err != nil {
		return SQLResult{}, err
	}
	result, err := read(queryCtx, session, tpl, args)
	result.Issuance = authority.Issuance()
	return result, err
}

// describes asks the database and believes it. The configuration check counts
// `$n` with a regular expression and cannot see a literal or a comment, so a
// template that passed there can still bind nothing here.
func describes(ctx context.Context, session SQLSession, tpl SQLTemplate) error {
	count, err := session.Describe(ctx, tpl.SQL)
	if err != nil {
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
	bytes := 0
	err := session.Query(ctx, tpl.SQL, args, func(row []byte) error {
		if len(out.Rows) >= tpl.MaxRows || bytes+len(row) > tpl.MaxBytes {
			out.Truncated = true
			return ErrResultTooLarge
		}
		out.Rows = append(out.Rows, row)
		bytes += len(row)
		return nil
	})
	if errors.Is(err, ErrResultTooLarge) {
		return out, nil
	}
	if err != nil {
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
	if shortest < 0 {
		return 0
	}
	return shortest
}

/*
revoke gives the lease back on a budget of its own.

Derived from the run's context with the cancellation removed, because the
moment a lease most needs handing back is the moment the run was cancelled —
using the same cancelled context would skip exactly the revocation it exists
for. The short TTL remains the backstop when this fails.
*/
func revoke(authority Authority) {
	ctx, cancel := context.WithTimeout(context.Background(), revocationGrace)
	defer cancel()
	_ = authority.Revoke(ctx)
}

func withoutCancel(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
