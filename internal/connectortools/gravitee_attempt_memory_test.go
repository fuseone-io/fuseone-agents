package connectortools

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/ticket"
)

type memoryGraviteeAttempts struct {
	mu       sync.Mutex
	attempts map[string]GraviteeAttempt
}

func newMemoryGraviteeAttempts() *memoryGraviteeAttempts {
	return &memoryGraviteeAttempts{attempts: make(map[string]GraviteeAttempt)}
}

func (m *memoryGraviteeAttempts) seed(external ticket.ExternalAttempt) error {
	attempt, err := graviteeAttemptFromExternal(external)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.attempts[attempt.IdemKey]; exists {
		return ticket.ErrAttemptConflict
	}
	m.attempts[attempt.IdemKey] = attempt
	return nil
}

func (m *memoryGraviteeAttempts) Get(_ context.Context, key string) (GraviteeAttempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	attempt, ok := m.attempts[key]
	if !ok {
		return GraviteeAttempt{}, ErrGraviteeAttemptNotFound
	}
	return attempt, nil
}

func (m *memoryGraviteeAttempts) ClaimDue(
	_ context.Context, owner string, now time.Time, lease time.Duration, limit int,
) ([]GraviteeAttempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.attempts))
	for key := range m.attempts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []GraviteeAttempt
	for _, key := range keys {
		a := m.attempts[key]
		if len(out) >= limit || a.Settled ||
			a.NextCheckAt.After(now) || (!a.ClaimedUntil.IsZero() && a.ClaimedUntil.After(now)) {
			continue
		}
		a.ClaimedBy, a.ClaimedUntil, a.UpdatedAt = owner, now.Add(lease), now
		m.attempts[key] = a
		out = append(out, a)
	}
	return out, nil
}

func (m *memoryGraviteeAttempts) Arm(
	_ context.Context, key, owner string, next, at time.Time,
) (GraviteeAttempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.attempts[key]
	if !ok || a.Status != GraviteeAttemptPrepared || a.ClaimedBy != owner {
		return GraviteeAttempt{}, ErrGraviteeAttemptClaim
	}
	a.Status, a.NextCheckAt, a.UpdatedAt = GraviteeAttemptPending, next, at
	a.ClaimedBy, a.ClaimedUntil = "", time.Time{}
	m.attempts[key] = a
	return a, nil
}

func (m *memoryGraviteeAttempts) Resolve(
	_ context.Context, in GraviteeAttemptResolution,
) (GraviteeAttempt, error) {
	if err := validateGraviteeResolution(in); err != nil {
		return GraviteeAttempt{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.attempts[in.IdemKey]
	if !ok || (in.ClaimedBy != "" && a.ClaimedBy != in.ClaimedBy) {
		return GraviteeAttempt{}, ErrGraviteeAttemptClaim
	}
	a.Status, a.Result, a.OutcomeCode = in.Status, in.Result, in.OutcomeCode
	a.Checks, a.NextCheckAt, a.UpdatedAt = a.Checks+1, in.NextCheckAt, in.At
	a.ClaimedBy, a.ClaimedUntil = "", time.Time{}
	m.attempts[in.IdemKey] = a
	return a, nil
}

func (m *memoryGraviteeAttempts) Settle(_ context.Context, key string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.attempts[key]
	if !ok || (a.Status != GraviteeAttemptConfirmed && a.Status != GraviteeAttemptTerminal) {
		return ErrGraviteeAttemptNotFound
	}
	a.Settled, a.NextCheckAt, a.UpdatedAt = true, time.Time{}, at
	m.attempts[key] = a
	return nil
}
