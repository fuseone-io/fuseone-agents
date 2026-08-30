package connectortools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
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

const maxTemplateTimeoutSeconds = 3600

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

var templateID = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func validateTemplates(instanceName string, templates []SQLTemplate) error {
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
	case tpl.MaxRows <= 0:
		return fmt.Errorf("connector: sql %s template %s needs a row limit", instanceName, tpl.ID)
	case tpl.MaxBytes <= 0:
		return fmt.Errorf("connector: sql %s template %s needs a byte limit", instanceName, tpl.ID)
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
		if param.Name == "" || seen[param.Name] {
			return fmt.Errorf("connector: sql %s template %s has an unnamed or duplicated parameter",
				instanceName, tpl.ID)
		}
		seen[param.Name] = true
	}
	return nil
}

var placeholder = regexp.MustCompile(`\$(\d+)`)

/*
validatePlaceholders makes the query and its parameters agree.

Every declared parameter is bound to exactly one placeholder, in order: $1 is
the first, $n is the nth, and there are no others. A template that declares two
and uses one leaves a supplied value with nowhere to go; one that reads $3 with
two declared reads a placeholder nobody described, which the driver would
report as an error on an approved query rather than here.
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
