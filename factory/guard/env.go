package guard

import (
	"fmt"
	"strconv"
	"strings"
)

// envReader keeps the first error, so LoadConfig reads as a list of fields.
type envReader struct {
	get func(string) string
	err error
}

func (e *envReader) fail(name, why string) {
	if e.err == nil {
		e.err = fmt.Errorf("%w: %s %s", ErrConfig, name, why)
	}
}

func (e *envReader) required(name string) string {
	v := strings.TrimSpace(e.get(name))
	if v == "" {
		e.fail(name, "is required")
	}
	return v
}

func (e *envReader) orDefault(name, def string) string {
	if v := strings.TrimSpace(e.get(name)); v != "" {
		return v
	}
	return def
}

// absent refuses a variable that belongs to another mode, so nobody believes
// it narrows something it does not.
func (e *envReader) absent(name, why string) {
	if strings.TrimSpace(e.get(name)) != "" {
		e.fail(name, why)
	}
}

func (e *envReader) id(name string) int64 {
	raw := e.required(name)
	if raw == "" {
		return 0
	}
	return e.positive(name, raw)
}

func (e *envReader) positive(name, raw string) int64 {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		e.fail(name, "must be a positive integer")
		return 0
	}
	return n
}

func (e *envReader) list(name string) []string {
	var out []string
	for _, item := range strings.Split(e.required(name), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		e.fail(name, "must name at least one entry")
	}
	return out
}

func (e *envReader) repo(name string) Repo {
	owner, repo, ok := strings.Cut(e.required(name), "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		e.fail(name, "must be owner/name")
		return Repo{}
	}
	return Repo{Owner: owner, Name: repo}
}

func (e *envReader) oneOf(name, value string, allowed ...string) {
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	e.fail(name, "must be one of "+strings.Join(allowed, ", "))
}

// pairs reads name=value,name=value. Every name must come from names, and
// every name and value may appear once; when complete, every name must be
// there. A value goes to check, which reports whether it is well formed.
func (e *envReader) pairs(name string, names []string, complete bool, check func(string) bool) map[string]string {
	out := map[string]string{}
	seen := map[string]bool{}
	for _, item := range strings.Split(e.required(name), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(item), "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch {
		case !ok || !check(value):
			e.fail(name, "must be a list of name=value with well-formed values")
		case !contains(names, key):
			e.fail(name, "names "+key+", which is not one of "+strings.Join(names, ", "))
		case out[key] != "" || seen[value]:
			e.fail(name, "repeats a name or a value")
		}
		out[key], seen[value] = value, true
	}
	if complete && len(out) != len(names) {
		e.fail(name, "must name every one of "+strings.Join(names, ", "))
	}
	return out
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func names[T ~string](set []T) []string {
	out := make([]string, len(set))
	for i, s := range set {
		out[i] = string(s)
	}
	return out
}
