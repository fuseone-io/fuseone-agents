package connectortools

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestSQLRuntime_recordsSafeIssuanceAndBoundedStageOutcomes(t *testing.T) {
	t.Parallel()

	metrics := &recordingSQLMetrics{}
	runtime := NewSQLRuntime(
		NewCredentialResolver(staticConfig(ready()), tokenFor(ready()), issuer()),
		&recordingSQLExecutor{},
	).WithMetrics(metrics)

	result, err := runtime.Run(
		context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.IssuanceOutcome != IssuanceSucceeded || result.Revocation != RevocationSucceeded {
		t.Fatalf("result = %+v, want successful issuance and revocation", result)
	}
	metrics.want(t, map[string]int{
		"issuance/succeeded":   1,
		"query/succeeded":      1,
		"revocation/succeeded": 1,
	})
}

func TestSQLRuntime_aMissingBindingRecordsARefusalWithoutInventingIssuance(t *testing.T) {
	t.Parallel()

	metrics := &recordingSQLMetrics{}
	runtime := NewSQLRuntime(
		NewCredentialResolver(staticConfig(nil), tokenFor(nil), issuer()),
		&recordingSQLExecutor{},
	).WithMetrics(metrics)

	result, err := runtime.Run(
		context.Background(), "app-x", "orders_by_customer", runScope(), params())
	if !errors.Is(err, ErrNoCredentialSource) {
		t.Fatalf("Run err = %v, want ErrNoCredentialSource", err)
	}
	if result.IssuanceOutcome != IssuanceRefused || result.Issuance.SQLInstance != "" {
		t.Fatalf("result = %+v, want a refusal without invented provenance", result)
	}
	metrics.want(t, map[string]int{"issuance/refused": 1})
}

func TestSQLRuntime_aPreflightRefusalDoesNotClaimTheDatabaseWasQueried(t *testing.T) {
	t.Parallel()

	metrics := &recordingSQLMetrics{}
	runtime := NewSQLRuntime(
		NewCredentialResolver(staticConfig(ready()), tokenFor(ready()), issuer()),
		&recordingSQLExecutor{},
	).WithMetrics(metrics)

	_, err := runtime.Run(
		context.Background(), "app-x", "not_registered", runScope(), params())
	if !errors.Is(err, ErrNoSuchTemplate) {
		t.Fatalf("Run err = %v, want ErrNoSuchTemplate", err)
	}
	metrics.want(t, map[string]int{
		"issuance/succeeded":   1,
		"revocation/succeeded": 1,
	})
}

type recordingSQLMetrics struct {
	mu     sync.Mutex
	counts map[string]int
}

func (m *recordingSQLMetrics) SQLRuntime(stage, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts == nil {
		m.counts = map[string]int{}
	}
	m.counts[stage+"/"+outcome]++
}

func (m *recordingSQLMetrics) want(t *testing.T, want map[string]int) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.counts) != len(want) {
		t.Fatalf("metrics = %#v, want %#v", m.counts, want)
	}
	for key, count := range want {
		if m.counts[key] != count {
			t.Fatalf("metrics[%q] = %d, want %d; all = %#v", key, m.counts[key], count, m.counts)
		}
	}
}
