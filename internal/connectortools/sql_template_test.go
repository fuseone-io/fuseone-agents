package connectortools

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func template() SQLTemplate {
	return SQLTemplate{
		ID:  "orders_by_customer",
		SQL: "select id, total from orders where customer_id = $1 and created_at >= $2",
		Parameters: []SQLParameter{
			{Name: "customer_id", Type: SQLParamText},
			{Name: "since", Type: SQLParamTimestamp},
		},
		TimeoutSeconds: 10, MaxRows: 200, MaxBytes: 64 << 10,
	}
}

func runnableSQL() Instance {
	instance := sqlInstance(area("acme", "platform"), vaultRole("prod"))
	instance.SQL.Driver = SQLDriverPostgres
	instance.SQL.Templates = []SQLTemplate{template()}
	return instance
}

// The driver is named, not guessed. A connector that inferred one from the
// port would connect to whatever answers there.
func TestValidateInstanceConfig_sqlNeedsADriverItSupports(t *testing.T) {
	t.Parallel()

	for _, driver := range []SQLDriver{"", "mysql", "postgres "} {
		instance := runnableSQL()
		instance.SQL.Driver = driver
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("driver %q was accepted", driver)
		}
	}
	if err := ValidateInstanceConfig(runnableSQL()); err != nil {
		t.Fatalf("postgres was refused: %v", err)
	}
}

/*
A dynamic credential never crosses a connection nobody verified.

The credential is short-lived, which bounds how long a stolen one is useful
and does nothing about it being read in flight. There is no field to turn this
off, so the test is that the config cannot express it — and the vault that
issues it is held to the same rule, because a token read over plain HTTP is
the same disclosure one step earlier.
*/
func TestValidateBindings_refusesIssuingOverAConnectionNobodyVerified(t *testing.T) {
	t.Parallel()

	plain := vaultInstance("prod", company("acme"))
	plain.Vault.Address = "http://vault.internal"

	err := ValidateBindings([]Instance{plain, runnableSQL()})
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("err = %v, want a refusal naming https", err)
	}
}

func TestValidateInstanceConfig_templatesAreCompleteAndBounded(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*SQLTemplate){
		"no id":            func(tpl *SQLTemplate) { tpl.ID = "" },
		"id with spaces":   func(tpl *SQLTemplate) { tpl.ID = "orders by customer" },
		"no sql":           func(tpl *SQLTemplate) { tpl.SQL = "" },
		"no timeout":       func(tpl *SQLTemplate) { tpl.TimeoutSeconds = 0 },
		"no row limit":     func(tpl *SQLTemplate) { tpl.MaxRows = 0 },
		"no byte limit":    func(tpl *SQLTemplate) { tpl.MaxBytes = 0 },
		"untyped param":    func(tpl *SQLTemplate) { tpl.Parameters[0].Type = "" },
		"unnamed param":    func(tpl *SQLTemplate) { tpl.Parameters[0].Name = "" },
		"duplicate param":  func(tpl *SQLTemplate) { tpl.Parameters[1].Name = "customer_id" },
		"timeout too long": func(tpl *SQLTemplate) { tpl.TimeoutSeconds = 3601 },
	}
	for name, break_ := range cases {
		instance := runnableSQL()
		break_(&instance.SQL.Templates[0])
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

/*
The SQL and its parameters have to agree, and the placeholders are the
agreement.

A template declaring two parameters and using one leaves a value the caller
supplies with nowhere to go; using three reads a placeholder nobody declared.
Both are configuration errors that would otherwise surface as a driver error
on an approved query.
*/
func TestValidateInstanceConfig_placeholdersMatchTheDeclaredParameters(t *testing.T) {
	t.Parallel()

	for name, sql := range map[string]string{
		"one placeholder for two parameters": "select 1 from orders where customer_id = $1",
		"a placeholder nobody declared":      "select 1 from orders where a = $1 and b = $2 and c = $3",
		"placeholders out of order":          "select 1 from orders where a = $2",
		"no placeholders at all":             "select 1 from orders",
	} {
		instance := runnableSQL()
		instance.SQL.Templates[0].SQL = sql
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestValidateInstanceConfig_templateIDsAreUniqueWithinAnInstance(t *testing.T) {
	t.Parallel()

	instance := runnableSQL()
	instance.SQL.Templates = append(instance.SQL.Templates, template())
	if err := ValidateInstanceConfig(instance); err == nil {
		t.Fatal("two templates with one id were accepted")
	}
}

// A named template is found by id and nothing else. The tool takes an id and
// parameters, so this is the only place SQL text is chosen.
func TestSQLConfig_templateIsFoundByIDAlone(t *testing.T) {
	t.Parallel()

	cfg := runnableSQL().SQL
	found, ok := cfg.Template("orders_by_customer")
	if !ok || found.SQL != template().SQL {
		t.Fatalf("Template = %+v, %v", found, ok)
	}
	if _, ok := cfg.Template("orders_by_customer "); ok {
		t.Error("a template was found by a name that is not its id")
	}
	// The template's own query text, which is the string a lookup by content
	// would match. Anything but the id resolving here is the arbitrary-SQL
	// path reopening from the other side.
	if _, ok := cfg.Template(template().SQL); ok {
		t.Error("a template resolved by its own SQL text")
	}
}

func TestValidateInstanceConfig_limitsHaveCeilings(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*Instance){
		"rows beyond the ceiling":  func(i *Instance) { i.SQL.Templates[0].MaxRows = 10_001 },
		"bytes beyond the ceiling": func(i *Instance) { i.SQL.Templates[0].MaxBytes = domain.DefaultContentLimit + 1 },
		"query longer than the cap": func(i *Instance) {
			i.SQL.Templates[0].SQL = "select '" + strings.Repeat("x", 16<<10) + "' where a = $1 and b = $2"
		},
		"too many parameters": func(i *Instance) {
			for n := range 40 {
				i.SQL.Templates[0].Parameters = append(i.SQL.Templates[0].Parameters,
					SQLParameter{Name: "p" + string(rune('a'+n%26)) + string(rune('a'+n/26)), Type: SQLParamText})
			}
		},
		"too many templates": func(i *Instance) {
			for n := range 70 {
				tpl := template()
				tpl.ID = "t" + string(rune('a'+n%26)) + string(rune('a'+n/26))
				i.SQL.Templates = append(i.SQL.Templates, tpl)
			}
		},
	}
	for name, break_ := range cases {
		instance := runnableSQL()
		break_(&instance)
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

/*
The placeholder check does not read SQL, and this is where that is written
down.

`$1` inside a literal or a comment counts here and binds nothing at the
database. The cases below are accepted today on purpose: the authority is the
executor, which must Describe the statement and compare the count the database
reports. When that lands, this test becomes the list of cases it has to catch.
*/
func TestValidatePlaceholders_isNotAuthoritativeAboutSQL(t *testing.T) {
	t.Parallel()

	for name, sql := range map[string]string{
		"placeholder inside a literal": "select '$1' from orders",
		"placeholder inside a comment": "select 1 from orders -- $1",
	} {
		instance := runnableSQL()
		instance.SQL.Templates[0].SQL = sql
		instance.SQL.Templates[0].Parameters = []SQLParameter{{Name: "p", Type: SQLParamText}}
		if err := ValidateInstanceConfig(instance); err != nil {
			t.Errorf("%s: refused here, so the executor no longer needs to catch it: %v", name, err)
		}
	}
}

/*
The byte ceiling is the content store's, not a number of its own.

A result larger than the store's limit is truncated to a prefix once stored,
so a template allowed to ask for more would have the executor hold bytes the
platform is about to drop. Asserted against the constant rather than a literal:
when the store's limit rises this must rise with it, and a literal here would
quietly stop matching.
*/
func TestTemplateByteCeiling_followsTheContentStore(t *testing.T) {
	t.Parallel()

	instance := runnableSQL()
	instance.SQL.Templates[0].MaxBytes = domain.DefaultContentLimit
	if err := ValidateInstanceConfig(instance); err != nil {
		t.Fatalf("the store's own limit was refused: %v", err)
	}
	instance.SQL.Templates[0].MaxBytes = domain.DefaultContentLimit + 1
	if err := ValidateInstanceConfig(instance); err == nil {
		t.Fatal("a template may ask for more than the store will keep")
	}
}

func TestValidateInstanceConfig_namesAreBoundedIdentifiers(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 65)
	for name, break_ := range map[string]func(*Instance){
		"long template id":  func(i *Instance) { i.SQL.Templates[0].ID = long },
		"long param name":   func(i *Instance) { i.SQL.Templates[0].Parameters[0].Name = long },
		"shouting param":    func(i *Instance) { i.SQL.Templates[0].Parameters[0].Name = "Customer" },
		"param with spaces": func(i *Instance) { i.SQL.Templates[0].Parameters[0].Name = "customer id" },
	} {
		instance := runnableSQL()
		break_(&instance)
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

/*
An enabled SQL instance without templates is a tool nobody can call.

Once the connector is runtime its operations appear on an agent's surface, and
every call is refused for naming a template that does not exist. Zero is
allowed while the instance is disabled, because that is what a configuration
looks like halfway through being written.
*/
func TestValidateInstanceConfig_anEnabledInstanceRegistersSomethingCallable(t *testing.T) {
	t.Parallel()

	instance := runnableSQL()
	instance.SQL.Templates = nil
	if err := ValidateInstanceConfig(instance); err == nil {
		t.Fatal("an enabled instance with no templates was accepted")
	}
	instance.Enabled = false
	if err := ValidateInstanceConfig(instance); err != nil {
		t.Fatalf("a disabled instance may be unfinished: %v", err)
	}
}
