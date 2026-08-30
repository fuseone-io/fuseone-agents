package connectortools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

/*
A registered template is the only SQL that runs.

The tool takes a template id and parameters. It does not take SQL, and there
is no field anywhere in this package where a model's text becomes a query —
that is the difference between a governed read and arbitrary execution, and it
has to be structural rather than a rule somebody remembers.
*/
type SQLDriver string

// SQLDriverPostgres is the first and only driver. Named rather than inferred:
// a connector that guessed from the port would connect to whatever answers
// there, with a credential minted for something else.
const SQLDriverPostgres SQLDriver = "postgres"

type SQLParamType string

const (
	SQLParamText      SQLParamType = "text"
	SQLParamInteger   SQLParamType = "integer"
	SQLParamNumber    SQLParamType = "number"
	SQLParamBoolean   SQLParamType = "boolean"
	SQLParamTimestamp SQLParamType = "timestamp"
)

type SQLParameter struct {
	Name string
	Type SQLParamType
}

/*
SQLTemplate is one query an administrator registered, with the limits it runs
under.

Timeout, rows and bytes are per template rather than per instance: the shape
of an answer is a property of the question, and a limit that fits a lookup by
key is the wrong limit for a report.
*/
type SQLTemplate struct {
	ID             string
	SQL            string
	Parameters     []SQLParameter
	TimeoutSeconds int
	MaxRows        int
	MaxBytes       int
}

/*
Ceilings, because a limit without one is not a limit.

An administrator writes these, so the numbers are not adversarial — but a row
limit of two billion protects neither the database that produces the rows nor
the memory that holds them, and it reads as a bound to whoever configured it.
Each is generous for the operations this connector exists for and refuses the
value that would only ever be a mistake.
*/
const (
	maxTemplateTimeoutSeconds = 3600
	maxTemplateRows           = 10_000
	// Aligned to the content store's own limit rather than chosen. A result
	// larger than that is truncated to a prefix once it is stored, so letting
	// a template ask for more would have the executor hold bytes in memory
	// that the platform is about to drop. When DefaultContentLimit rises, this
	// rises with it — which is why it is derived and not a second number.
	maxTemplateBytes        = domain.DefaultContentLimit
	maxTemplateSQLBytes     = 16 << 10
	maxTemplateParameters   = 32
	maxTemplatesPerInstance = 64
	// Names are identifiers, not prose. Sixty-four templates times thirty-two
	// parameters is a lot of room for a name nobody bounded to inflate a
	// setting and, later, a tool schema the model has to read.
	maxTemplateNameBytes = 64
)

// Template is the query for an id, and only for an id. Nothing here searches
// by text: a caller that could pass SQL and have it matched would have found
// the arbitrary-execution path this design closes.
func (c SQLConfig) Template(id string) (SQLTemplate, bool) {
	for _, tpl := range c.Templates {
		if tpl.ID == id {
			return tpl, true
		}
	}
	return SQLTemplate{}, false
}

// templateID and paramName are the same shape: a lower-case identifier, which
// is what both become in a tool schema. Bounded here as well as in the
// contract, because the domain is what refuses when a setting arrives by any
// other route.
var templateID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

var paramName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

/*
An enabled SQL instance registers at least one template.

Zero is not a smaller configuration, it is a tool with no id anyone can call:
once the connector is runtime, the operations still appear on an agent's
surface and every call is refused for naming a template that does not exist.
A disabled instance may be empty, because that is what half-written
configuration looks like while somebody is writing it.
*/
func validateTemplates(instanceName string, enabled bool, templates []SQLTemplate) error {
	if enabled && len(templates) == 0 {
		return fmt.Errorf(
			"connector: sql %s must register at least one template, or be disabled while it is written",
			instanceName)
	}
	if len(templates) > maxTemplatesPerInstance {
		return fmt.Errorf("connector: sql %s registers more than %d templates",
			instanceName, maxTemplatesPerInstance)
	}
	seen := map[string]bool{}
	for _, tpl := range templates {
		if err := validateTemplate(instanceName, tpl); err != nil {
			return err
		}
		if seen[tpl.ID] {
			return fmt.Errorf("connector: sql %s has two templates called %q",
				instanceName, tpl.ID)
		}
		seen[tpl.ID] = true
	}
	return nil
}

func validateTemplate(instanceName string, tpl SQLTemplate) error {
	switch {
	case !templateID.MatchString(tpl.ID):
		return fmt.Errorf("connector: sql %s has a template with an invalid id %q",
			instanceName, tpl.ID)
	case strings.TrimSpace(tpl.SQL) == "":
		return fmt.Errorf("connector: sql %s template %s has no query", instanceName, tpl.ID)
	case tpl.TimeoutSeconds <= 0 || tpl.TimeoutSeconds > maxTemplateTimeoutSeconds:
		return fmt.Errorf("connector: sql %s template %s needs a timeout between 1 and %d seconds",
			instanceName, tpl.ID, maxTemplateTimeoutSeconds)
	case tpl.MaxRows <= 0 || tpl.MaxRows > maxTemplateRows:
		return fmt.Errorf("connector: sql %s template %s needs a row limit between 1 and %d",
			instanceName, tpl.ID, maxTemplateRows)
	case tpl.MaxBytes <= 0 || tpl.MaxBytes > maxTemplateBytes:
		return fmt.Errorf("connector: sql %s template %s needs a byte limit between 1 and %d",
			instanceName, tpl.ID, maxTemplateBytes)
	case len(tpl.SQL) > maxTemplateSQLBytes:
		return fmt.Errorf("connector: sql %s template %s is longer than %d bytes",
			instanceName, tpl.ID, maxTemplateSQLBytes)
	case len(tpl.Parameters) > maxTemplateParameters:
		return fmt.Errorf("connector: sql %s template %s declares more than %d parameters",
			instanceName, tpl.ID, maxTemplateParameters)
	}
	if err := validateParameters(instanceName, tpl); err != nil {
		return err
	}
	return validatePlaceholders(instanceName, tpl)
}

func validateParameters(instanceName string, tpl SQLTemplate) error {
	seen := map[string]bool{}
	for _, param := range tpl.Parameters {
		switch param.Type {
		case SQLParamText, SQLParamInteger, SQLParamNumber, SQLParamBoolean, SQLParamTimestamp:
		default:
			return fmt.Errorf("connector: sql %s template %s parameter %q has no declared type",
				instanceName, tpl.ID, param.Name)
		}
		if !paramName.MatchString(param.Name) || seen[param.Name] {
			return fmt.Errorf(
				"connector: sql %s template %s parameter names must be lower-case identifiers of at most %d bytes, and unique",
				instanceName, tpl.ID, maxTemplateNameBytes)
		}
		seen[param.Name] = true
	}
	return nil
}

var placeholder = regexp.MustCompile(`\$(\d+)`)

/*
validatePlaceholders catches the common disagreement early, and is not the
authority.

Every declared parameter should bind one placeholder, in order. This finds the
mistake an administrator actually makes — declaring two and using one — at the
moment they make it, instead of on the first approved query.

What it cannot do is read SQL. `$1` inside a string literal or a comment counts
here and is invisible to the database, so `select '$1'` with one parameter
declared passes this and would bind nothing. Writing a SQL parser to close that
would be maintaining a second, worse implementation of the driver's own.

The authority is the executor: it must Describe the statement and compare the
parameter count the database reports before running anything. Until that
exists, this is a smell check, and the test below pins the gap so it is a known
limitation rather than a believed guarantee.
*/
func validatePlaceholders(instanceName string, tpl SQLTemplate) error {
	found := map[int]bool{}
	for _, match := range placeholder.FindAllStringSubmatch(tpl.SQL, -1) {
		index, err := strconv.Atoi(match[1])
		if err != nil {
			return fmt.Errorf("connector: sql %s template %s has an unreadable placeholder",
				instanceName, tpl.ID)
		}
		found[index] = true
	}
	if len(found) != len(tpl.Parameters) {
		return fmt.Errorf(
			"connector: sql %s template %s declares %d parameters and uses %d placeholders",
			instanceName, tpl.ID, len(tpl.Parameters), len(found))
	}
	for i := range tpl.Parameters {
		if !found[i+1] {
			return fmt.Errorf("connector: sql %s template %s never uses $%d",
				instanceName, tpl.ID, i+1)
		}
	}
	return nil
}
