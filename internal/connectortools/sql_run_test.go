package connectortools

import (
	"context"
	"encoding/json"
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
	rows       []json.RawMessage
	columns    []string
	queryErr   error
	closeBlock time.Duration
	cancelOn   context.CancelFunc
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

func (s *sqlSpy) Query(ctx context.Context, _ string, args []any, sink SQLSink) error {
	s.args = args
	if deadline, ok := ctx.Deadline(); ok {
		s.deadline = time.Until(deadline)
	}
	if err := sink.Columns(s.columns); err != nil {
		return err
	}
	// Cancelling here, mid-read, is the case a run actually meets: a resolver
	// would never mint a credential for a context that was already cancelled.
	if s.cancelOn != nil {
		s.cancelOn()
		return ctx.Err()
	}
	if s.queryErr != nil {
		return s.queryErr
	}
	for _, value := range s.rows {
		if err := sink.Row(value); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqlSpy) Close(ctx context.Context) error {
	s.closed++
	s.order = append(s.order, "close")
	// A close that blocks past its budget, which is what pgx warns about. If
	// the runtime gave it the same context as the revocation, or none, the
	// hand-back below would never run.
	if s.closeBlock > 0 {
		select {
		case <-time.After(s.closeBlock):
		case <-ctx.Done():
		}
	}
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

	spy := &sqlSpy{params: 2, rows: []json.RawMessage{json.RawMessage(`["a"]`)}}
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

	rows := make([]json.RawMessage, 500)
	for i := range rows {
		rows[i] = json.RawMessage(`["` + strings.Repeat("x", 100) + `"]`)
	}
	// The exact counts are asserted below against a serialised result, because
	// an estimate of the envelope is exactly what this stopped being.
	want := map[string]int{"rows": 3}
	for name, limit := range map[string]func(*SQLTemplate){
		"rows":  func(tpl *SQLTemplate) { tpl.MaxRows = 3 },
		"bytes": func(tpl *SQLTemplate) { tpl.MaxBytes = minTemplateBytes },
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
		if !result.Truncated {
			t.Fatalf("%s: the read was not cut short", name)
		}
		if expected, ok := want[name]; ok && len(result.Rows) != expected {
			t.Errorf("%s: rows = %d, want %d", name, len(result.Rows), expected)
		}
		// The claim the limit actually makes: what gets stored fits. Asserting
		// a row count would only pin whatever the envelope estimate happened
		// to be.
		if name == "bytes" {
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if len(encoded) > minTemplateBytes {
				t.Errorf("stored answer is %d bytes, past the template's %d",
					len(encoded), minTemplateBytes)
			}
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

	ctx, cancel := context.WithCancel(context.Background())
	spy := &sqlSpy{params: 2, cancelOn: cancel}
	rt, vault := runtime(t, spy)
	_, err := rt.Run(ctx, "app-x", "orders_by_customer", runScope(), params())
	// Cancellation keeps its identity all the way out: a stopped run and a
	// database that refused are different facts, and only the second is a
	// reason to look at the database.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled preserved", err)
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

	spy := &sqlSpy{params: 2, rows: []json.RawMessage{json.RawMessage(`["row"]`)}}
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

/*
Each of the three deadlines wins on its own.

Only the lease had an accuser, so removing either of the other two comparisons
left every test green. A minimum is not proved by one branch: it is proved by
each input being the smallest at least once.
*/
func TestSQLRuntime_deadlineIsTheEarliestOfThree(t *testing.T) {
	t.Parallel()

	for name, arrange := range map[string]struct {
		ttl      int
		timeout  int
		runLimit time.Duration
		want     time.Duration
	}{
		"the template is shortest": {ttl: 600, timeout: 4, runLimit: 0, want: 5 * time.Second},
		"the lease is shortest":    {ttl: 9, timeout: 600, runLimit: 0, want: 5 * time.Second},
		"the run is shortest":      {ttl: 600, timeout: 600, runLimit: 2 * time.Second, want: 3 * time.Second},
	} {
		spy := &sqlSpy{params: 2}
		vault := issuer()
		vault.ttl = arrange.ttl
		instances := ready()
		tpl := template()
		tpl.TimeoutSeconds = arrange.timeout
		instances[1].SQL.Templates = []SQLTemplate{tpl}
		rt := NewSQLRuntime(NewCredentialResolver(
			staticConfig(instances), tokenFor(instances), vault), spy)

		ctx := context.Background()
		if arrange.runLimit > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, arrange.runLimit)
			defer cancel()
		}
		if _, err := rt.Run(ctx, "app-x", "orders_by_customer", runScope(), params()); err != nil {
			t.Fatalf("%s: Run: %v", name, err)
		}
		if spy.deadline >= arrange.want {
			t.Errorf("%s: deadline = %v, want under %v", name, spy.deadline, arrange.want)
		}
	}
}

/*
A lease that expires before the query could run refuses before the connection.

Opening with a context that is already spent makes the database look
unreachable when the truth is a clock, and it costs a connection to learn
something the numbers already said.
*/
func TestSQLRuntime_refusesALeaseTooShortToUse(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 2}
	vault := issuer()
	vault.ttl = 3
	instances := ready()
	instances[1].SQL.Templates = []SQLTemplate{template()}
	rt := NewSQLRuntime(NewCredentialResolver(
		staticConfig(instances), tokenFor(instances), vault), spy)

	result, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if !errors.Is(err, ErrLeaseTooShort) {
		t.Fatalf("err = %v, want the lease refused", err)
	}
	if spy.opened != 0 {
		t.Errorf("the database was reached with a spent budget")
	}
	if result.Revocation != RevocationSucceeded {
		t.Errorf("revocation = %q, want the unused lease handed back", result.Revocation)
	}
}

/*
A close that blocks does not swallow the revocation.

pgx documents that Close can block. Given the same context as the hand-back, or
none at all, a stuck connection would keep a credential alive until its TTL —
which is the failure the close-then-revoke ordering exists to prevent, arriving
through the ordering itself.
*/
func TestSQLRuntime_revokesEvenWhenCloseBlocks(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 2, closeBlock: time.Minute}
	rt, vault := runtime(t, spy)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Run never returned; a blocked close swallowed the hand-back")
	}
	if vault.revoked == "" {
		t.Fatal("a blocked close left the lease alive")
	}
}

// What happened to the lease is recorded, and what Vault said is not. A failed
// hand-back and a successful one are the difference between a credential that
// expires in minutes and one nobody knows is open.
func TestSQLRuntime_recordsTheRevocationOutcome(t *testing.T) {
	t.Parallel()

	failing := issuer()
	failing.revokeErr = errors.New("vault: 403 " + credentialCanary)
	instances := ready()
	instances[1].SQL.Templates = []SQLTemplate{template()}
	spy := &sqlSpy{params: 2, rows: []json.RawMessage{json.RawMessage(`["a"]`)}}
	rt := NewSQLRuntime(NewCredentialResolver(
		staticConfig(instances), tokenFor(instances), failing), spy)

	result, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Revocation != RevocationFailed {
		t.Fatalf("revocation = %q, want failed", result.Revocation)
	}
	leaks(t, result)
}

/*
The stored answer is JSON an auditor can read, and the limit counts it.

Rows as [][]byte serialise to base64, so a governed result would be an encoding
of an encoding. And a limit that counted only row bytes would report itself
respected while the envelope and the separators carried the payload past it.
*/
func TestSQLResult_isReadableJSONAndBoundedAsStored(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{
		params:  2,
		columns: []string{"id", "total"},
		rows:    []json.RawMessage{json.RawMessage(`[1,"9.90"]`)},
	}
	rt, _ := runtime(t, spy)
	result, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"rows":[[1,"9.90"]]`) {
		t.Fatalf("stored answer = %s, want rows as JSON arrays", encoded)
	}
	if !strings.Contains(string(encoded), `"columns":["id","total"]`) {
		t.Fatalf("stored answer = %s, want named columns", encoded)
	}
}

/*
Provenance survives every way a run can fail after the credential exists.

A run that failed at Open or Describe recorded a credential it had certainly
been issued as if it never had one — the issuance was filled in only after a
successful read. The lease is still handed back either way, so the record
without it was a credential that existed, was returned, and left no trace of
either.
*/
func TestSQLRuntime_recordsIssuanceOnEveryPathPastTheCredential(t *testing.T) {
	t.Parallel()

	for name, arrange := range map[string]func(*sqlSpy){
		"open failed":     func(s *sqlSpy) { s.openErr = errors.New("dial") },
		"describe failed": func(s *sqlSpy) { s.params = 0 },
		"query failed":    func(s *sqlSpy) { s.queryErr = errors.New("pq: boom") },
	} {
		spy := &sqlSpy{params: 2}
		arrange(spy)
		rt, _ := runtime(t, spy)
		result, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
		if err == nil {
			t.Errorf("%s: no error", name)
		}
		if result.Issuance.VaultInstance != "prod" || result.Issuance.LeaseTTLSeconds == 0 {
			t.Errorf("%s: issuance = %+v, want the credential that was minted", name, result.Issuance)
		}
		if result.Revocation != RevocationSucceeded {
			t.Errorf("%s: revocation = %q, want the lease handed back", name, result.Revocation)
		}
	}
}

// A failure before anything is issued reports a value the enum declares. An
// empty string is not "not attempted", it is a field nobody set.
func TestSQLRuntime_reportsNotAttemptedBeforeAnyCredentialExists(t *testing.T) {
	t.Parallel()

	spy := &sqlSpy{params: 2}
	rt := NewSQLRuntime(NewCredentialResolver(
		staticConfig(nil), tokenFor(nil), issuer()), spy)
	result, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if err == nil {
		t.Fatal("an unconfigured instance resolved")
	}
	if result.Revocation != RevocationNotAttempted {
		t.Fatalf("revocation = %q, want not_attempted", result.Revocation)
	}
}

/*
The byte limit is measured against what is stored, whatever the columns are.

An estimate is wrong in one of two directions and the wrong one is
under-counting, which puts a payload past the content store while reporting the
limit kept. Long column names are the cheapest way to prove the envelope is
measured rather than assumed.
*/
func TestSQLRuntime_boundsTheStoredAnswerWithLongColumnNames(t *testing.T) {
	t.Parallel()

	rows := make([]json.RawMessage, 200)
	for i := range rows {
		rows[i] = json.RawMessage(`["` + strings.Repeat("y", 80) + `"]`)
	}
	columns := make([]string, 12)
	for i := range columns {
		columns[i] = strings.Repeat("column_name_", 6) + string(rune('a'+i))
	}
	spy := &sqlSpy{params: 2, columns: columns, rows: rows}
	tpl := template()
	// Big enough that the columns fit and small enough that they are a real
	// share of the budget, rather than a rounding error against 64 KiB.
	tpl.MaxBytes = 4 << 10
	instances := ready()
	instances[1].SQL.Templates = []SQLTemplate{tpl}
	rt := NewSQLRuntime(NewCredentialResolver(
		staticConfig(instances), tokenFor(instances), issuer()), spy)

	result, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(encoded) > tpl.MaxBytes {
		t.Fatalf("stored answer is %d bytes, past the template's %d",
			len(encoded), tpl.MaxBytes)
	}
	if !result.Truncated {
		t.Fatal("nothing was cut, so this proves nothing about the bound")
	}
}

/*
An answer whose columns alone pass the limit is refused, not truncated.

There is nothing left to drop: returning it would be the limit reporting
itself kept while the content store truncates behind it. Columns are not known
until the query runs, so this is the first moment the mismatch between a wide
result and a small template limit can be said at all.
*/
func TestSQLRuntime_refusesAnAnswerWhoseShapeCannotFit(t *testing.T) {
	t.Parallel()

	columns := make([]string, 20)
	for i := range columns {
		columns[i] = strings.Repeat("very_long_column_", 8) + string(rune('a'+i))
	}
	spy := &sqlSpy{params: 2, columns: columns, rows: []json.RawMessage{json.RawMessage(`["x"]`)}}
	tpl := template()
	tpl.MaxBytes = minTemplateBytes
	instances := ready()
	instances[1].SQL.Templates = []SQLTemplate{tpl}
	rt := NewSQLRuntime(NewCredentialResolver(
		staticConfig(instances), tokenFor(instances), issuer()), spy)

	result, err := rt.Run(context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Truncated || len(result.Rows) != 0 {
		t.Fatalf("rows = %d truncated = %v, want nothing returned", len(result.Rows), result.Truncated)
	}
	// And the lease still goes back: a refusal is not a reason to keep one.
	if result.Revocation != RevocationSucceeded {
		t.Errorf("revocation = %q, want the lease handed back", result.Revocation)
	}
}
