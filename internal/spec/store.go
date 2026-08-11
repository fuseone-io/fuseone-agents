package spec

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"

	"github.com/fuseone/agents/internal/domain"
)

var ErrNotFound = errors.New("spec: no such agent version")

// Store holds every version of every agent.
//
// Versions accumulate rather than replace. A run pins to the version that
// started it and keeps reading that one for its whole life, so publishing
// never changes what an in-flight run is doing (PRD DE-09).
type Store struct {
	mu sync.RWMutex
	// byVersion is the authoritative map: an exact version always resolves,
	// even after it stops being current.
	byVersion map[versionKey]Spec
	// current points at the latest published version of each agent.
	current map[domain.AgentID]domain.VersionID
	// history preserves publication order, for the console's version list.
	history map[domain.AgentID][]domain.VersionID
}

type versionKey struct {
	agent   domain.AgentID
	version domain.VersionID
}

func NewStore() *Store {
	return &Store{
		byVersion: make(map[versionKey]Spec),
		current:   make(map[domain.AgentID]domain.VersionID),
		history:   make(map[domain.AgentID][]domain.VersionID),
	}
}

// Publish records a version and makes it current.
//
// Publishing the same bytes twice is a no-op rather than an error: the version
// is the content digest, so a redundant publish cannot produce a different
// version and there is nothing to reconcile.
func (s *Store) Publish(spec Spec) error {
	if spec.ID == "" || spec.Version == "" {
		return fmt.Errorf("%w: a spec needs an id and a version", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := versionKey{spec.ID, spec.Version}
	if _, exists := s.byVersion[key]; !exists {
		s.byVersion[key] = spec
		s.history[spec.ID] = append(s.history[spec.ID], spec.Version)
	}
	s.current[spec.ID] = spec.Version
	return nil
}

// Get returns an exact version. An empty version means the current one.
func (s *Store) Get(agent domain.AgentID, version domain.VersionID) (Spec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if version == "" {
		v, ok := s.current[agent]
		if !ok {
			return Spec{}, fmt.Errorf("%w: %s", ErrNotFound, agent)
		}
		version = v
	}

	spec, ok := s.byVersion[versionKey{agent, version}]
	if !ok {
		return Spec{}, fmt.Errorf("%w: %s@%s", ErrNotFound, agent, version)
	}
	return spec, nil
}

// Agents lists the published agents, sorted.
func (s *Store) Agents() []domain.AgentID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.AgentID, 0, len(s.current))
	for id := range s.current {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// Versions lists an agent's versions in publication order.
func (s *Store) Versions(agent domain.AgentID) []domain.VersionID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.history[agent])
}

// LoadDir publishes every agent definition under root.
//
// Definitions live in the customer's own repository and are applied from CI,
// so an author gets code review, diff and rollback without ever being asked to
// know what a commit is (PRD DE-10).
func (s *Store) LoadDir(ctx context.Context, fsys fs.FS, root string) (int, error) {
	var loaded int

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		spec, err := Parse(path, data)
		if err != nil {
			// One malformed definition must not silently drop out of the
			// catalogue — the author would see nothing and assume it loaded.
			return err
		}
		if err := s.Publish(spec); err != nil {
			return err
		}
		loaded++
		return nil
	})

	return loaded, err
}
