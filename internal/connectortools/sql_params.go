package connectortools

import (
	"fmt"
	"strconv"
	"time"
)

/*
bindParameters turns what a caller supplied into the ordered arguments a
statement takes.

Ordered by the template, not by the map: the query says `$1`, and which value
that is has to come from the registered order rather than from whatever order
a JSON object happened to arrive in.

Every value is converted to its declared type here and passed to the driver as
a bound argument. Nothing is formatted into the query — there is no code path
in this package that concatenates a value into SQL, which is what makes
injection structurally absent rather than carefully avoided.
*/
func bindParameters(tpl SQLTemplate, supplied map[string]any) ([]any, error) {
	args := make([]any, 0, len(tpl.Parameters))
	for _, param := range tpl.Parameters {
		raw, ok := supplied[param.Name]
		if !ok {
			return nil, fmt.Errorf("connector: template %s needs %s", tpl.ID, param.Name)
		}
		value, err := convert(param, raw)
		if err != nil {
			return nil, err
		}
		args = append(args, value)
	}
	// Extra values are refused rather than ignored. A caller that sent one
	// believes it reached the query, and silently dropping it would answer a
	// different question than the one they asked.
	if len(supplied) != len(tpl.Parameters) {
		return nil, fmt.Errorf("connector: template %s takes %d parameters and %d were supplied",
			tpl.ID, len(tpl.Parameters), len(supplied))
	}
	return args, nil
}

/*
convert reads one value as its declared type.

Deliberately narrow about what counts. A model produces JSON, so a number
arrives as float64 and a timestamp as text; both are converted here, and
anything that does not convert is refused rather than passed along for the
driver to interpret.
*/
func convert(param SQLParameter, raw any) (any, error) {
	switch param.Type {
	case SQLParamText:
		if value, ok := raw.(string); ok {
			return value, nil
		}
	case SQLParamInteger:
		return asInteger(raw)
	case SQLParamNumber:
		if value, ok := asFloat(raw); ok {
			return value, nil
		}
	case SQLParamBoolean:
		if value, ok := raw.(bool); ok {
			return value, nil
		}
	case SQLParamTimestamp:
		if text, ok := raw.(string); ok {
			value, err := time.Parse(time.RFC3339, text)
			if err == nil {
				return value, nil
			}
		}
	}
	return nil, fmt.Errorf("connector: %s is not a %s", param.Name, param.Type)
}

// asInteger refuses a number with a fractional part rather than truncating it.
// A caller that sent 1.5 for a row id meant something, and it was not 1.
func asInteger(raw any) (any, error) {
	switch value := raw.(type) {
	case int:
		return int64(value), nil
	case int64:
		return value, nil
	case float64:
		if value == float64(int64(value)) {
			return int64(value), nil
		}
	case string:
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed, nil
		}
	}
	return nil, fmt.Errorf("connector: %v is not a whole number", raw)
}

func asFloat(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		return parsed, err == nil
	}
	return 0, false
}
