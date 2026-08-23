package worker

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// MetricsRegistry renders low-cardinality worker metrics.
//
// The ledger is still the diagnostic record. Metrics answer only whether the
// pool is moving: no run id, agent id, tool name, error text, or user input is
// allowed to become a Prometheus label.
type MetricsRegistry struct {
	mu    sync.Mutex
	pools map[string]*PoolMetrics
}

func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{pools: map[string]*PoolMetrics{}}
}

// Pool returns the metrics handle one worker pool writes to.
func (r *MetricsRegistry) Pool(name string, slots int) *PoolMetrics {
	r.mu.Lock()
	defer r.mu.Unlock()

	p := &PoolMetrics{
		pool:     name,
		slots:    slots,
		claims:   map[string]uint64{},
		advances: map[string]uint64{},
		failures: map[failureMetric]uint64{},
		parks:    map[string]uint64{},
	}
	r.pools[name] = p
	return p
}

// ServeHTTP emits Prometheus text exposition.
func (r *MetricsRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/metrics" {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	snap := r.snapshot()
	fmt.Fprintln(w, "# HELP fuseone_worker_slots Configured concurrent run slots in this worker process.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_slots gauge")
	for _, p := range snap {
		fmt.Fprintf(w, "fuseone_worker_slots{pool=%s} %d\n", label(p.pool), p.slots)
	}

	fmt.Fprintln(w, "# HELP fuseone_worker_claims_total Queue claim attempts by result.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_claims_total counter")
	for _, p := range snap {
		for _, key := range sortedKeys(p.claims) {
			fmt.Fprintf(w, "fuseone_worker_claims_total{pool=%s,result=%s} %d\n",
				label(p.pool), label(key), p.claims[key])
		}
	}

	fmt.Fprintln(w, "# HELP fuseone_worker_advances_total Completed worker turns by resulting phase.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_advances_total counter")
	for _, p := range snap {
		for _, key := range sortedKeys(p.advances) {
			fmt.Fprintf(w, "fuseone_worker_advances_total{pool=%s,phase=%s} %d\n",
				label(p.pool), label(key), p.advances[key])
		}
	}

	fmt.Fprintln(w, "# HELP fuseone_worker_advance_failures_total Planner or runner failures by stable reason.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_advance_failures_total counter")
	for _, p := range snap {
		for _, key := range sortedFailureKeys(p.failures) {
			fmt.Fprintf(w, "fuseone_worker_advance_failures_total{pool=%s,reason=%s,parked=%s} %d\n",
				label(p.pool), label(key.reason), label(fmt.Sprint(key.parked)), p.failures[key])
		}
	}

	fmt.Fprintln(w, "# HELP fuseone_worker_parks_total Runs parked by the worker supervisor by reason.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_parks_total counter")
	for _, p := range snap {
		for _, key := range sortedKeys(p.parks) {
			fmt.Fprintf(w, "fuseone_worker_parks_total{pool=%s,reason=%s} %d\n",
				label(p.pool), label(key), p.parks[key])
		}
	}
}

type poolSnapshot struct {
	pool     string
	slots    int
	claims   map[string]uint64
	advances map[string]uint64
	failures map[failureMetric]uint64
	parks    map[string]uint64
}

func (r *MetricsRegistry) snapshot() []poolSnapshot {
	r.mu.Lock()
	pools := make([]*PoolMetrics, 0, len(r.pools))
	for _, p := range r.pools {
		pools = append(pools, p)
	}
	r.mu.Unlock()

	sort.Slice(pools, func(i, j int) bool { return pools[i].pool < pools[j].pool })
	out := make([]poolSnapshot, 0, len(pools))
	for _, p := range pools {
		out = append(out, p.snapshot())
	}
	return out
}

type failureMetric struct {
	reason string
	parked bool
}

// PoolMetrics is safe to share across the worker's slot goroutines.
type PoolMetrics struct {
	mu       sync.Mutex
	pool     string
	slots    int
	claims   map[string]uint64
	advances map[string]uint64
	failures map[failureMetric]uint64
	parks    map[string]uint64
}

func (m *PoolMetrics) Claim(result string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.claims[result]++
}

func (m *PoolMetrics) Advance(phase string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advances[phase]++
}

func (m *PoolMetrics) AdvanceFailure(reason string, parked bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failures[failureMetric{reason: reason, parked: parked}]++
}

func (m *PoolMetrics) Park(reason string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.parks[reason]++
}

func (m *PoolMetrics) snapshot() poolSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return poolSnapshot{
		pool:     m.pool,
		slots:    m.slots,
		claims:   copyStringCounters(m.claims),
		advances: copyStringCounters(m.advances),
		failures: copyFailureCounters(m.failures),
		parks:    copyStringCounters(m.parks),
	}
}

func copyStringCounters(in map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyFailureCounters(in map[failureMetric]uint64) map[failureMetric]uint64 {
	out := make(map[failureMetric]uint64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sortedKeys(m map[string]uint64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedFailureKeys(m map[failureMetric]uint64) []failureMetric {
	keys := make([]failureMetric, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].reason != keys[j].reason {
			return keys[i].reason < keys[j].reason
		}
		return !keys[i].parked && keys[j].parked
	})
	return keys
}

func label(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
