package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
)

const (
	MinToolDedupeWindowSeconds = 1
	MaxToolDedupeWindowSeconds = 365 * 24 * 60 * 60
	MaxToolDedupeArgPaths      = 8
)

var dedupeArgPathPattern = regexp.MustCompile(
	`^[A-Za-z_][A-Za-z0-9_-]*(?:\.[A-Za-z_][A-Za-z0-9_-]*)*$`,
)

// ToolDedupe is the Curator's declaration of what makes two effectful calls
// the same external act across runs.
//
// Company, area, agent and tool are always platform-owned key prefixes. The
// Curator names only stable fields inside the proposed arguments, so a model
// cannot choose a broader scope and two tenants cannot learn about each
// other's effects through dedupe.
type ToolDedupe struct {
	WindowSeconds int
	ArgPaths      []string
}

func (d ToolDedupe) Enabled() bool {
	return d.WindowSeconds != 0 || len(d.ArgPaths) != 0
}

func (d ToolDedupe) Clone() ToolDedupe {
	d.ArgPaths = append([]string(nil), d.ArgPaths...)
	return d
}

func (d ToolDedupe) Validate() error {
	if !d.Enabled() {
		return nil
	}
	if d.WindowSeconds < MinToolDedupeWindowSeconds ||
		d.WindowSeconds > MaxToolDedupeWindowSeconds {
		return fmt.Errorf("dedupe window must be between %d and %d seconds",
			MinToolDedupeWindowSeconds, MaxToolDedupeWindowSeconds)
	}
	if len(d.ArgPaths) == 0 {
		return fmt.Errorf("dedupe needs at least one argument path")
	}
	if len(d.ArgPaths) > MaxToolDedupeArgPaths {
		return fmt.Errorf("dedupe may name at most %d argument paths", MaxToolDedupeArgPaths)
	}
	seen := make(map[string]struct{}, len(d.ArgPaths))
	for _, path := range d.ArgPaths {
		if !dedupeArgPathPattern.MatchString(path) {
			return fmt.Errorf("dedupe argument path %q is not a stable dotted field path", path)
		}
		if _, ok := seen[path]; ok {
			return fmt.Errorf("dedupe argument path %q is duplicated", path)
		}
		seen[path] = struct{}{}
	}
	return nil
}

// Fingerprint hashes only the Curator-declared argument fields. It deliberately
// ignores undeclared fields: correlation ids and timestamps in raw arguments
// must not defeat cross-run dedupe when they are not part of the external act.
func (d ToolDedupe) Fingerprint(args []byte) (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	root, err := decodeDedupeArgs(args)
	if err != nil {
		return "", fmt.Errorf("dedupe arguments are not JSON: %w", err)
	}

	paths := append([]string(nil), d.ArgPaths...)
	slices.Sort(paths)

	h := sha256.New()
	for _, path := range paths {
		value, ok := valueAtPath(root, path)
		if !ok {
			return "", fmt.Errorf("dedupe argument path %q is absent", path)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("dedupe argument path %q: %w", path, err)
		}
		_, _ = h.Write([]byte(path))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(encoded)
		_, _ = h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))[:16], nil
}

func decodeDedupeArgs(args []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.UseNumber()

	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("contains more than one JSON value")
	}
	return root, nil
}

func valueAtPath(root any, path string) (any, bool) {
	current := root
	for _, segment := range splitDottedPath(path) {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func splitDottedPath(path string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] != '.' {
			continue
		}
		parts = append(parts, path[start:i])
		start = i + 1
	}
	return append(parts, path[start:])
}
