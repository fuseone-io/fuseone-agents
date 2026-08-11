package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// How a condition reads the request and compares what it finds.
//
// Every operator here has to survive being rendered back as a sentence the
// author checks by eye, which is what keeps the vocabulary short. "Matches
// this regular expression" is not such a sentence.

func (c Condition) holds(in PolicyInput) bool {
	actual, present := fieldOf(c.Field, in)

	switch c.Op {
	case OpEquals:
		return present && equal(actual, c.Value)
	case OpNotEquals:
		// A field that is not there is not equal to anything, so a `ne` rule
		// holds. The alternative — an absent field failing every comparison —
		// would make "deny unless marked reviewed" silently never fire.
		return !present || !equal(actual, c.Value)
	case OpGreaterThan, OpLessThan:
		return present && compares(actual, c.Value, c.Op)
	case OpContains:
		return present && strings.Contains(strings.ToLower(actual), strings.ToLower(c.Value))
	case OpIn:
		return present && inList(actual, c.Value)
	default:
		// An operator nobody implemented must not silently pass. A rule that
		// cannot be evaluated is a rule that does not hold.
		return false
	}
}

// fieldOf reads one field out of the input.
func fieldOf(field string, in PolicyInput) (string, bool) {
	switch field {
	case "tool.id":
		return string(in.Tool), true
	case "tool.effect":
		return in.Effect.String(), true
	case "agent.id":
		return string(in.Agent), true
	case "scope.company":
		return string(in.Scope.Company), true
	case "scope.area":
		return string(in.Scope.Area), true
	case "data.taint":
		return strings.Join(in.Labels, ","), len(in.Labels) > 0
	}
	if path, ok := strings.CutPrefix(field, "args."); ok {
		return argOf(in.Args, path)
	}
	return "", false
}

// argOf reads a path out of the proposed arguments.
//
// One level of nesting, dotted. Anything deeper is a rule nobody can read back
// as a sentence, which is the line this vocabulary is drawn at.
func argOf(args []byte, path string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	var decoded map[string]any
	if err := json.Unmarshal(args, &decoded); err != nil {
		return "", false
	}

	var current any = decoded
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		if current, ok = object[part]; !ok {
			return "", false
		}
	}
	return render(current), true
}

// render turns a JSON value into the text a condition compares against.
func render(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return ""
	case []any:
		// A list compares by length, which is what `args.rows > 100` means
		// when rows is the rows themselves rather than a count.
		return strconv.Itoa(len(v))
	default:
		return fmt.Sprint(v)
	}
}

func equal(actual, want string) bool {
	return strings.EqualFold(actual, want)
}

// compares reads both sides as numbers when they are, and as text otherwise.
// "1000" must not be less than "100" because it sorts that way.
func compares(actual, want, op string) bool {
	left, leftErr := strconv.ParseFloat(actual, 64)
	right, rightErr := strconv.ParseFloat(want, 64)
	if leftErr != nil || rightErr != nil {
		if op == OpGreaterThan {
			return actual > want
		}
		return actual < want
	}
	if op == OpGreaterThan {
		return left > right
	}
	return left < right
}

// inList holds when the actual value is one of a comma-separated set, or when
// any of the actual values is. `data.taint in untrusted,personal` has to hold
// for a call carrying both.
func inList(actual, want string) bool {
	wanted := strings.Split(want, ",")
	for _, have := range strings.Split(actual, ",") {
		for _, w := range wanted {
			if equal(strings.TrimSpace(have), strings.TrimSpace(w)) {
				return true
			}
		}
	}
	return false
}

// globMatches supports a trailing star and nothing else. `crm.*` and `crm.reply`
// are both readable; `*.re?ly` is a pattern nobody can check by eye.
func globMatches(pattern, value string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(value, prefix)
	}
	return pattern == value
}

func contains[T comparable](haystack []T, needle T) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
