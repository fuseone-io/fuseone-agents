package connectortools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type sqlSpy struct {
	opened     int
	closed     int
	describe   int
	params     int
	describeAt string
	args       []any
	rows       [][]byte
	queryErr   error
	openErr    error
	order      []string
	deadline   time.Duration
}

func (s *sqlSpy) Open(_ context.Context, _ SQLConfig, _ Credential) (SQLSession, error) {
	s.opened++
	if s.openErr != nil {
		return nil, s.openErr
	}
	return s, nil
}

func (s *sqlSpy) Describe(_ context.Context, sql string) (int, error) {
	s.describe++
	s.describeAt = sql
	return s.params, nil
}

func (s *sqlSpy) Query(ctx context.Context, _ string, args []any, row func([]byte) error) error {
	s.args = args
	if deadline, ok := ctx.Deadline(); ok {
		s.deadline = time.Until(deadline)
	}
	if s.queryErr != nil {
		return s.queryErr
	}
	for _, value := range s.rows {
		if err := row(value); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqlSpy) Close(context.Context) error {
	s.closed++
	s.order = append(s.order, "close")
	return nil
}

type revokingIssuer struct {
	*vaultIssuer
	order *[]string
}

func (r revokingIssuer) RevokeLease(ctx context.Context, cfg VaultConfig, token, leaseID string) error {
	*r.order = append(*r.order, "revoke")
	return r.vaultIssuer.RevokeLease(ctx, cfg, token, leaseID)
}

func runtime(t *testing.T, spy *sqlSpy) (*SQLRuntime, *vaultIssuer) {
	t.Helper()
	vault := issuer()
	instances := ready()
	instances[1].SQL.Templates = []SQLTemplate{template()}
	resolver := NewCredentialResolver(staticConfig(instances), tokenFor(instances),
		revokingIssuer{vaultIssuer: vault, order: &spy.order})
	return NewSQLRuntime(resolver, spy), vault
}

func params() map[string]any {
	return map[string]any{"customer_id": "cus_1", "since": "2026-08-01T00:00:00Z"}
}

/*
The connection closes before the lease is given back.

A lease handed back while a connection still holds it is a connection the
database may keep serving, so the order is not a preference. Both are deferred,
so a failure past the credential takes the same path as a return.
*/
func TestSQLRuntime_closesBeforeRevoking(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 2, rows: [][]byte{[]byte("a")}}
	rt, _ := runtime(t, spy)
	if _, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Join(spy.order, ",") != "close,revoke" {
		t.Fatalf("order = %v, want close then revoke", spy.order)
	}
}

// A request that cannot run still hands back the lease it was given: waiting
// out a TTL for a template that does not exist is a credential alive for
// nothing.
func TestSQLRuntime_revokesEvenWhenTheRequestNeverRuns(t *testing.T) {
	t.Parallel()

	for name, run := range map[string]func(*SQLRuntime) error{
		"no such template": func(rt *SQLRuntime) error {
			_, err := rt.Run(context.Background(), "app-x", "nope", runScope(), params())
			return err
		},
		"parameter missing": func(rt *SQLRuntime) error {
			_, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(),
				map[string]any{"customer_id": "cus_1"})
			return err
		},
	} {
		spy := &sqlSpy{params: 2}
		rt, vault := runtime(t, spy)
		if err := run(rt); err == nil {
			t.Errorf("%s: ran anyway", name)
		}
		if vault.revoked == "" {
			t.Errorf("%s: the lease was left to expire", name)
		}
		if spy.opened != 0 {
			t.Errorf("%s: the database was reached", name)
		}
	}
}

/*
The database decides how many parameters a statement takes.

The configuration check counts `$n` with a regular expression and cannot see a
literal or a comment, so a template that passed there can bind nothing here.
This is the case that check is documented as unable to catch.
*/
func TestSQLRuntime_believesTheDatabaseAboutParameters(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 0}
	rt, _ := runtime(t, spy)
	_, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if !errors.Is(err, ErrParameterMismatch) {
		t.Fatalf("err = %v, want a parameter mismatch", err)
	}
	if spy.describeAt != template().SQL {
		t.Errorf("described %q, want the registered query", spy.describeAt)
	}
}

// Values are bound in the template's order and converted to their declared
// types. Nothing is formatted into the query anywhere in this package.
func TestSQLRuntime_bindsParametersInTheRegisteredOrder(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 2}
	rt, _ := runtime(t, spy)
	if _, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(spy.args) != 2 || spy.args[0] != "cus_1" {
		t.Fatalf("args = %#v, want the declared order", spy.args)
	}
	if _, ok := spy.args[1].(time.Time); !ok {
		t.Fatalf("args[1] = %T, want the declared timestamp type", spy.args[1])
	}
}

func TestSQLRuntime_stopsAtTheTemplateLimits(t *testing.T) {
	t.Parallel()

	rows := make([][]byte, 500)
	for i := range rows {
		rows[i] = []byte(strings.Repeat("x", 100))
	}
	for name, limit := range map[string]func(*SQLTemplate){
		"rows":  func(tpl *SQLTemplate) { tpl.MaxRows = 3 },
		"bytes": func(tpl *SQLTemplate) { tpl.MaxBytes = 250 },
	} {
		spy := &sqlSpy{params: 2, rows: rows}
		vault := issuer()
		instances := ready()
		tpl := template()
		limit(&tpl)
		instances[1].SQL.Templates = []SQLTemplate{tpl}
		rt := NewSQLRuntime(NewCredentialResolver(
			staticConfig(instances), tokenFor(instances), vault), spy)

		result, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
		if err != nil {
			t.Fatalf("%s: Run: %v", name, err)
		}
		if !result.Truncated || len(result.Rows) >= len(rows) {
			t.Fatalf("%s: rows = %d truncated = %v, want the read cut short",
				name, len(result.Rows), result.Truncated)
		}
	}
}

/*
The query's deadline is the earliest of three.

The template's bound, what the run has left, and the credential's TTL less a
margin. The lease is the one nobody thinks of: a statement cut off by an
expiring credential looks like a database failure and is a clock.
*/
func TestSQLRuntime_deadlineIsBoundedByTheLease(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 2}
	vault := issuer()
	vault.ttl = 8
	instances := ready()
	instances[1].SQL.Templates = []SQLTemplate{template()}
	rt := NewSQLRuntime(NewCredentialResolver(
		staticConfig(instances), tokenFor(instances), vault), spy)

	if _, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Eight seconds of lease, five of margin: three, not the template's ten.
	if spy.deadline > 4*time.Second {
		t.Fatalf("deadline = %v, want it bounded by the lease", spy.deadline)
	}
}

// A cancelled run still gives its lease back. Using the run's own context to
// revoke would skip exactly the case the revocation exists for.
func TestSQLRuntime_revokesAfterCancellation(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 2, queryErr: context.Canceled}
	rt, vault := runtime(t, spy)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rt.Run(ctx, "app-x", "orders_by_customer", runScope(), params()); err == nil {
		t.Fatal("a cancelled run returned no error")
	}
	if vault.revoked == "" {
		t.Fatal("a cancelled run left its lease alive")
	}
	if spy.closed != 1 {
		t.Fatalf("closed = %d, want the connection closed once", spy.closed)
	}
}

/*
Nothing the database or the driver said reaches the caller.

A driver error can quote the statement, the bound parameters and sometimes a
value from a row. The template is named because an operator needs to know which
one; everything else is the canary.
*/
func TestSQLRuntime_errorsCarryNothingFromTheDatabase(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 2, queryErr: errors.New("pq: near \"" + credentialCanary + "\"")}
	rt, _ := runtime(t, spy)
	_, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if err == nil {
		t.Fatal("a failing query returned no error")
	}
	leaks(t, err.Error())
	if !strings.Contains(err.Error(), "orders_by_customer") {
		t.Errorf("err = %v, want the template named", err)
	}
}

// The credential reaches the executor and nothing else. The result a run
// records carries safe provenance and rows, never the authority they cost.
func TestSQLRuntime_resultCarriesNoCredential(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 2, rows: [][]byte{[]byte("row")}}
	rt, _ := runtime(t, spy)
	result, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	leaks(t, result)
	if result.Issuance.VaultInstance != "prod" {
		t.Fatalf("issuance = %+v, want safe provenance", result.Issuance)
	}
}
