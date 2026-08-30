package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

var (
	ErrNoSuchTemplate    = errors.New("connector: no such registered template")
	ErrParameterMismatch = errors.New(
		"connector: the database reads a different number of parameters")
	ErrResultTooLarge      = errors.New("connector: the result passed the template's limit")
	ErrAnswerShapeTooLarge = errors.New(
		"connector: the answer's shape exceeds the template's byte limit")
	ErrSinkOutOfOrder     = errors.New("connector: the executor produced rows out of order")
	ErrCredentialInResult = errors.New(
		"connector: the database result contains execution authority")
	ErrSQLContractChanged = errors.New(
		"connector: the registered SQL contract changed after the Gate decision")
)

// SQLSession is one connection, opened for one operation. Close must release
// the transport even when its graceful context expires, because revocation
// follows it and must not race a connection that still holds the lease.
type SQLSession interface {
	// Describe is authoritative about parameter count; configuration validation
	// deliberately does not attempt to parse SQL.
	Describe(ctx context.Context, sql string) (int, error)
	// Query runs read-only and announces columns before streaming JSON rows.
	Query(ctx context.Context, sql string, args []any, sink SQLSink) error
	Close(ctx context.Context) error
}

type SQLSink interface {
	Columns(names []string) error
	Row(row json.RawMessage) error
}

type SQLExecutor interface {
	Open(
		ctx context.Context, cfg SQLConfig, credential Credential, timeout time.Duration,
	) (SQLSession, error)
}

type RevocationOutcome string

const (
	RevocationNotAttempted RevocationOutcome = "not_attempted"
	RevocationSucceeded    RevocationOutcome = "succeeded"
	RevocationFailed       RevocationOutcome = "failed"
)

// IssuanceOutcome records whether this execution obtained authority, without
// carrying any Vault response or credential material into governed content.
type IssuanceOutcome string

const (
	IssuanceNotAttempted IssuanceOutcome = "not_attempted"
	IssuanceSucceeded    IssuanceOutcome = "succeeded"
	IssuanceRefused      IssuanceOutcome = "refused"
	IssuanceFailed       IssuanceOutcome = "failed"
)

// SQLResult is the governed answer. MaxBytes bounds its encoded form, not just
// the rows, so columns and safe provenance cannot silently exceed the limit.
type SQLResult struct {
	Columns         []string          `json:"columns"`
	Rows            []json.RawMessage `json:"rows"`
	Truncated       bool              `json:"truncated"`
	IssuanceOutcome IssuanceOutcome   `json:"issuanceOutcome"`
	Issuance        Issuance          `json:"issuance"`
	Revocation      RevocationOutcome `json:"revocation"`
}

type SQLRuntime struct {
	resolver *CredentialResolver
	executor SQLExecutor
	metrics  SQLRuntimeMetrics
}

// SQLRuntimeMetrics receives only bounded stage/outcome pairs. Instance,
// template, run, error and credential data are deliberately not parameters.
type SQLRuntimeMetrics interface {
	SQLRuntime(stage, outcome string)
}

func NewSQLRuntime(resolver *CredentialResolver, executor SQLExecutor) *SQLRuntime {
	return &SQLRuntime{resolver: resolver, executor: executor}
}

// RunBound executes only the server-owned contract the Gate evaluated. The
// runtime checks after reading current settings, closing the race between an
// approval and an administrator changing the registered query or target.
func (r *SQLRuntime) RunBound(
	ctx context.Context, instance, templateID, contractDigest string,
	scope domain.Scope, params map[string]any,
) (SQLResult, error) {
	if contractDigest == "" {
		r.observe("query", "refused")
		return SQLResult{
			IssuanceOutcome: IssuanceNotAttempted,
			Revocation:      RevocationNotAttempted,
		}, ErrSQLContractChanged
	}
	return r.run(ctx, instance, templateID, contractDigest, scope, params)
}

func (r *SQLRuntime) WithMetrics(metrics SQLRuntimeMetrics) *SQLRuntime {
	r.metrics = metrics
	return r
}
