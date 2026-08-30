package connectortools

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

const (
	revocationGrace  = 5 * time.Second
	closeGrace       = 3 * time.Second
	minTemplateBytes = 1024
	ttlSafetyMargin  = 5 * time.Second
)

/*
Run is the whole authority lifecycle.

Authority is resolved, one connection is opened, the database describes the
registered statement, rows are bounded while they arrive, the connection is
closed, and the lease is returned. Nothing reaches the database before Vault
answers, and nothing survives one execution.
*/
func (r *SQLRuntime) run(
	ctx context.Context, instance, templateID, expectedContract string,
	scope domain.Scope, params map[string]any,
) (result SQLResult, err error) {
	result = SQLResult{
		IssuanceOutcome: IssuanceNotAttempted,
		Revocation:      RevocationNotAttempted,
	}
	authority, err := r.resolver.Resolve(ctx, instance, scope)
	if err != nil {
		result.IssuanceOutcome = issuanceOutcome(err)
		r.observe("issuance", runtimeOutcome(err))
		return result, err
	}
	result.IssuanceOutcome = IssuanceSucceeded
	result.Issuance = authority.Issuance()
	r.observe("issuance", "succeeded")

	target := authority.Target()
	tpl, ok := target.Template(templateID)
	if !ok {
		result.Revocation = r.revoke(ctx, authority)
		if expectedContract != "" {
			return result, ErrSQLContractChanged
		}
		return result, fmt.Errorf("%w: %s", ErrNoSuchTemplate, templateID)
	}
	if expectedContract != "" {
		current, _ := sqlContractDigest(target, authority.lease.config, templateID)
		if current != expectedContract {
			result.Revocation = r.revoke(ctx, authority)
			return result, ErrSQLContractChanged
		}
	}
	args, err := bindParameters(tpl, params)
	if err != nil {
		result.Revocation = r.revoke(ctx, authority)
		return result, err
	}

	budget := effectiveTimeout(ctx, tpl, authority.Issuance())
	if budget <= 0 {
		result.Revocation = r.revoke(ctx, authority)
		return result, ErrLeaseTooShort
	}
	queryCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	defer func() { r.observe("query", runtimeOutcome(err)) }()

	session, err := r.executor.Open(queryCtx, target, authority.Credential(), budget)
	if err != nil {
		result.Revocation = r.revoke(ctx, authority)
		return result, reached(err, instance)
	}
	// Close before revoke, always. Both use contexts without the run's
	// cancellation because cancellation is when returning authority matters.
	defer func() {
		closeCtx, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), closeGrace)
		_ = session.Close(closeCtx)
		cancelClose()
		result.Revocation = r.revoke(ctx, authority)
	}()

	if err := describes(queryCtx, session, tpl); err != nil {
		return result, err
	}
	err = read(queryCtx, session, tpl, args, authority.Credential(), &result)
	return result, err
}

func issuanceOutcome(err error) IssuanceOutcome {
	if errors.Is(err, ErrNoCredentialSource) {
		return IssuanceRefused
	}
	return IssuanceFailed
}

func runtimeOutcome(err error) string {
	switch {
	case err == nil:
		return "succeeded"
	case errors.Is(err, ErrNoCredentialSource), errors.Is(err, ErrNoSuchTemplate),
		errors.Is(err, ErrParameterMismatch), errors.Is(err, ErrLeaseTooShort),
		errors.Is(err, ErrSQLContractChanged):
		return "refused"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "failed"
	}
}

func (r *SQLRuntime) observe(stage, outcome string) {
	if r != nil && r.metrics != nil {
		r.metrics.SQLRuntime(stage, outcome)
	}
}

func (r *SQLRuntime) revoke(ctx context.Context, authority Authority) RevocationOutcome {
	outcome := revoke(ctx, authority)
	r.observe("revocation", string(outcome))
	return outcome
}

// reached keeps cancellation recognisable and removes driver details, which
// may contain a DSN or parameter value.
func reached(err error, instance string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("connector: sql %s could not be reached", instance)
}

// describes asks the database and believes it. Configuration validation can
// catch common placeholder mistakes but deliberately does not parse SQL.
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

// effectiveTimeout is the earliest of the template limit, run deadline and
// credential TTL less its safety margin.
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

var ErrLeaseTooShort = errors.New("connector: the credential expires too soon to run this template")

// revoke keeps the run's trace and logger while removing its cancellation.
// Only the outcome escapes; a Vault message can name paths, roles and leases.
func revoke(ctx context.Context, authority Authority) RevocationOutcome {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), revocationGrace)
	defer cancel()
	if err := authority.Revoke(ctx); err != nil {
		return RevocationFailed
	}
	return RevocationSucceeded
}
