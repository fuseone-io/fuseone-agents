package worker

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/egressmetrics"
	"github.com/fuseone/agents/internal/tools"
)

// MetricsRegistry renders low-cardinality worker metrics.
//
// The ledger is still the diagnostic record. Metrics answer only whether the
// pool is moving: no run id, agent id, tool name, error text, or user input is
// allowed to become a Prometheus label.
type MetricsRegistry struct {
	mu                     sync.Mutex
	pools                  map[string]*PoolMetrics
	mcpToolCalls           map[mcpToolMetric]uint64
	mcpReservationRefusals map[string]uint64
	channelSweeps          map[channelSweepMetric]uint64
	channelFailures        map[channelFailureMetric]uint64
	channelItems           map[string]uint64
	stdioEgressDenials     map[string]uint64
	memoryFinds            map[memoryFindMetric]memoryFindCounters
}

func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		pools:                  map[string]*PoolMetrics{},
		mcpToolCalls:           map[mcpToolMetric]uint64{},
		mcpReservationRefusals: map[string]uint64{},
		channelSweeps:          map[channelSweepMetric]uint64{},
		channelFailures:        map[channelFailureMetric]uint64{},
		channelItems:           map[string]uint64{},
		stdioEgressDenials:     map[string]uint64{},
		memoryFinds:            map[memoryFindMetric]memoryFindCounters{},
	}
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
	renderWorkerMetrics(w, snap.pools)
	renderMCPMetrics(w, snap)
	renderChannelMetrics(w, snap)
	renderStdioEgressMetrics(w, snap)
	renderMemoryMetrics(w, snap)
}

func renderWorkerMetrics(w http.ResponseWriter, snap []poolSnapshot) {
	renderWorkerSlots(w, snap)
	renderWorkerClaims(w, snap)
	renderWorkerAdvances(w, snap)
	renderWorkerFailures(w, snap)
	renderWorkerParks(w, snap)
	renderRuntimeEfficiencyMetrics(w, snap)
}

func renderWorkerSlots(w http.ResponseWriter, snap []poolSnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_worker_slots Configured concurrent run slots in this worker process.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_slots gauge")
	for _, p := range snap {
		fmt.Fprintf(w, "fuseone_worker_slots{pool=%s} %d\n", label(p.pool), p.slots)
	}
}

func renderWorkerClaims(w http.ResponseWriter, snap []poolSnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_worker_claims_total Queue claim attempts by result.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_claims_total counter")
	for _, p := range snap {
		for _, key := range sortedKeys(p.claims) {
			fmt.Fprintf(w, "fuseone_worker_claims_total{pool=%s,result=%s} %d\n",
				label(p.pool), label(key), p.claims[key])
		}
	}
}

func renderWorkerAdvances(w http.ResponseWriter, snap []poolSnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_worker_advances_total Completed worker turns by resulting phase.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_advances_total counter")
	for _, p := range snap {
		for _, key := range sortedKeys(p.advances) {
			fmt.Fprintf(w, "fuseone_worker_advances_total{pool=%s,phase=%s} %d\n",
				label(p.pool), label(key), p.advances[key])
		}
	}
}

func renderWorkerFailures(w http.ResponseWriter, snap []poolSnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_worker_advance_failures_total Planner or runner failures by stable reason.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_advance_failures_total counter")
	for _, p := range snap {
		for _, key := range sortedFailureKeys(p.failures) {
			fmt.Fprintf(w, "fuseone_worker_advance_failures_total{pool=%s,reason=%s,parked=%s} %d\n",
				label(p.pool), label(key.reason), label(fmt.Sprint(key.parked)), p.failures[key])
		}
	}
}

func renderWorkerParks(w http.ResponseWriter, snap []poolSnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_worker_parks_total Runs parked by the worker supervisor by reason.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_parks_total counter")
	for _, p := range snap {
		for _, key := range sortedKeys(p.parks) {
			fmt.Fprintf(w, "fuseone_worker_parks_total{pool=%s,reason=%s} %d\n",
				label(p.pool), label(key), p.parks[key])
		}
	}
}

func renderMCPMetrics(w http.ResponseWriter, snap registrySnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_mcp_tool_calls_total MCP tool calls by result and stable code.")
	fmt.Fprintln(w, "# TYPE fuseone_mcp_tool_calls_total counter")
	for _, key := range sortedMCPToolKeys(snap.mcpToolCalls) {
		fmt.Fprintf(w, "fuseone_mcp_tool_calls_total{result=%s,code=%s,cached=%s} %d\n",
			label(key.result), label(key.code), label(key.cached), snap.mcpToolCalls[key])
	}

	fmt.Fprintln(w, "# HELP fuseone_mcp_reservation_refusals_total MCP calls refused before leaving the worker.")
	fmt.Fprintln(w, "# TYPE fuseone_mcp_reservation_refusals_total counter")
	for _, key := range sortedKeys(snap.mcpReservationRefusals) {
		fmt.Fprintf(w, "fuseone_mcp_reservation_refusals_total{code=%s} %d\n",
			label(key), snap.mcpReservationRefusals[key])
	}
}

func renderChannelMetrics(w http.ResponseWriter, snap registrySnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_channel_sweeps_total Channel sweeps by task and result.")
	fmt.Fprintln(w, "# TYPE fuseone_channel_sweeps_total counter")
	for _, key := range sortedChannelSweepKeys(snap.channelSweeps) {
		fmt.Fprintf(w, "fuseone_channel_sweeps_total{task=%s,result=%s} %d\n",
			label(key.task), label(key.result), snap.channelSweeps[key])
	}

	fmt.Fprintln(w, "# HELP fuseone_channel_failures_total Channel sweep failures by task and stable code.")
	fmt.Fprintln(w, "# TYPE fuseone_channel_failures_total counter")
	for _, key := range sortedChannelFailureKeys(snap.channelFailures) {
		fmt.Fprintf(w, "fuseone_channel_failures_total{task=%s,code=%s} %d\n",
			label(key.task), label(key.code), snap.channelFailures[key])
	}

	fmt.Fprintln(w, "# HELP fuseone_channel_items_total Channel items handled by task.")
	fmt.Fprintln(w, "# TYPE fuseone_channel_items_total counter")
	for _, key := range sortedKeys(snap.channelItems) {
		fmt.Fprintf(w, "fuseone_channel_items_total{task=%s} %d\n",
			label(key), snap.channelItems[key])
	}
}

func renderStdioEgressMetrics(w http.ResponseWriter, snap registrySnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_stdio_egress_denials_total Stdio MCP proxy denials by stable code.")
	fmt.Fprintln(w, "# TYPE fuseone_stdio_egress_denials_total counter")
	for _, key := range sortedKeys(snap.stdioEgressDenials) {
		fmt.Fprintf(w, "fuseone_stdio_egress_denials_total{code=%s} %d\n",
			label(key), snap.stdioEgressDenials[key])
	}
}

func renderMemoryMetrics(w http.ResponseWriter, snap registrySnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_memory_find_total Memory find tool calls by result and whether assertions were omitted by the response budget.")
	fmt.Fprintln(w, "# TYPE fuseone_memory_find_total counter")
	for _, key := range sortedMemoryFindKeys(snap.memoryFinds) {
		counters := snap.memoryFinds[key]
		fmt.Fprintf(w, "fuseone_memory_find_total{result=%s,omitted=%s} %d\n",
			label(key.result), label(key.omitted), counters.calls)
	}

	fmt.Fprintln(w, "# HELP fuseone_memory_find_duration_seconds Memory find latency accumulated by result.")
	fmt.Fprintln(w, "# TYPE fuseone_memory_find_duration_seconds summary")
	for _, key := range sortedMemoryFindKeys(snap.memoryFinds) {
		counters := snap.memoryFinds[key]
		fmt.Fprintf(w, "fuseone_memory_find_duration_seconds_sum{result=%s,omitted=%s} %.6f\n",
			label(key.result), label(key.omitted), float64(counters.durationMicros)/1_000_000)
		fmt.Fprintf(w, "fuseone_memory_find_duration_seconds_count{result=%s,omitted=%s} %d\n",
			label(key.result), label(key.omitted), counters.calls)
	}

	fmt.Fprintln(w, "# HELP fuseone_memory_find_returned_assertions_total Memory assertions returned to the model.")
	fmt.Fprintln(w, "# TYPE fuseone_memory_find_returned_assertions_total counter")
	for _, key := range sortedMemoryFindKeys(snap.memoryFinds) {
		counters := snap.memoryFinds[key]
		fmt.Fprintf(w, "fuseone_memory_find_returned_assertions_total{result=%s,omitted=%s} %d\n",
			label(key.result), label(key.omitted), counters.returned)
	}

	fmt.Fprintln(w, "# HELP fuseone_memory_find_omitted_assertions_total Memory assertions omitted from the tool response by the byte budget.")
	fmt.Fprintln(w, "# TYPE fuseone_memory_find_omitted_assertions_total counter")
	for _, key := range sortedMemoryFindKeys(snap.memoryFinds) {
		counters := snap.memoryFinds[key]
		fmt.Fprintf(w, "fuseone_memory_find_omitted_assertions_total{result=%s,omitted=%s} %d\n",
			label(key.result), label(key.omitted), counters.omitted)
	}
}

type poolSnapshot struct {
	pool     string
	slots    int
	claims   map[string]uint64
	advances map[string]uint64
	failures map[failureMetric]uint64
	parks    map[string]uint64
	runtime  runtimeCounters
}

type registrySnapshot struct {
	pools                  []poolSnapshot
	mcpToolCalls           map[mcpToolMetric]uint64
	mcpReservationRefusals map[string]uint64
	channelSweeps          map[channelSweepMetric]uint64
	channelFailures        map[channelFailureMetric]uint64
	channelItems           map[string]uint64
	stdioEgressDenials     map[string]uint64
	memoryFinds            map[memoryFindMetric]memoryFindCounters
}

func (r *MetricsRegistry) snapshot() registrySnapshot {
	r.mu.Lock()
	pools := make([]*PoolMetrics, 0, len(r.pools))
	for _, p := range r.pools {
		pools = append(pools, p)
	}
	snap := registrySnapshot{
		mcpToolCalls:           copyMCPToolCounters(r.mcpToolCalls),
		mcpReservationRefusals: copyStringCounters(r.mcpReservationRefusals),
		channelSweeps:          copyChannelSweepCounters(r.channelSweeps),
		channelFailures:        copyChannelFailureCounters(r.channelFailures),
		channelItems:           copyStringCounters(r.channelItems),
		stdioEgressDenials:     copyStringCounters(r.stdioEgressDenials),
		memoryFinds:            copyMemoryFindCounters(r.memoryFinds),
	}
	r.mu.Unlock()

	sort.Slice(pools, func(i, j int) bool { return pools[i].pool < pools[j].pool })
	snap.pools = make([]poolSnapshot, 0, len(pools))
	for _, p := range pools {
		snap.pools = append(snap.pools, p.snapshot())
	}
	return snap
}

type failureMetric struct {
	reason string
	parked bool
}

type mcpToolMetric struct {
	result string
	code   string
	cached string
}

type channelSweepMetric struct {
	task   string
	result string
}

type channelFailureMetric struct {
	task string
	code string
}

type memoryFindMetric struct {
	result  string
	omitted string
}

type memoryFindCounters struct {
	calls          uint64
	durationMicros uint64
	returned       uint64
	omitted        uint64
}

const metricOther = "other"

var (
	allowedMCPResults = map[string]bool{
		"error": true,
		"ok":    true,
	}
)

func (r *MetricsRegistry) MCPToolCall(result, code string, cached bool) {
	if r == nil {
		return
	}
	result = boundedMetricValue(result, allowedMCPResults)
	code = tools.MCPMetricCode(code)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mcpToolCalls[mcpToolMetric{result: result, code: code, cached: fmt.Sprint(cached)}]++
}

func (r *MetricsRegistry) MCPReservationRefused(code string) {
	if r == nil {
		return
	}
	code = tools.MCPMetricCode(code)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mcpReservationRefusals[code]++
}

func (r *MetricsRegistry) ChannelSweep(task, result string, items int) {
	if r == nil {
		return
	}
	task = channel.MetricTask(task)
	result = channel.MetricResult(result)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channelSweeps[channelSweepMetric{task: task, result: result}]++
	if items > 0 {
		r.channelItems[task] += uint64(items)
	}
}

func (r *MetricsRegistry) ChannelFailure(task, code string) {
	if r == nil {
		return
	}
	task = channel.MetricTask(task)
	code = channel.MetricCode(code)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channelFailures[channelFailureMetric{task: task, code: code}]++
}

func (r *MetricsRegistry) StdioEgressDenial(code string) {
	if r == nil {
		return
	}
	code = egressmetrics.Code(code)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stdioEgressDenials[code]++
}

func (r *MetricsRegistry) MemoryFind(duration time.Duration, returned int, omitted int, failed bool) {
	if r == nil {
		return
	}
	result := "ok"
	if failed {
		result = "error"
	}
	key := memoryFindMetric{result: result, omitted: fmt.Sprint(omitted > 0)}
	r.mu.Lock()
	defer r.mu.Unlock()
	counters := r.memoryFinds[key]
	counters.calls++
	if duration > 0 {
		counters.durationMicros += uint64(duration.Microseconds())
	}
	if returned > 0 {
		counters.returned += uint64(returned)
	}
	if omitted > 0 {
		counters.omitted += uint64(omitted)
	}
	r.memoryFinds[key] = counters
}

func boundedMetricValue(value string, allowed map[string]bool) string {
	if allowed[value] {
		return value
	}
	return metricOther
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
	runtime  runtimeCounters
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
		runtime:  m.runtime,
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

func copyMCPToolCounters(in map[mcpToolMetric]uint64) map[mcpToolMetric]uint64 {
	out := make(map[mcpToolMetric]uint64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyChannelSweepCounters(in map[channelSweepMetric]uint64) map[channelSweepMetric]uint64 {
	out := make(map[channelSweepMetric]uint64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyChannelFailureCounters(in map[channelFailureMetric]uint64) map[channelFailureMetric]uint64 {
	out := make(map[channelFailureMetric]uint64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyMemoryFindCounters(in map[memoryFindMetric]memoryFindCounters) map[memoryFindMetric]memoryFindCounters {
	out := make(map[memoryFindMetric]memoryFindCounters, len(in))
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

func sortedMCPToolKeys(m map[mcpToolMetric]uint64) []mcpToolMetric {
	keys := make([]mcpToolMetric, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].result != keys[j].result {
			return keys[i].result < keys[j].result
		}
		if keys[i].code != keys[j].code {
			return keys[i].code < keys[j].code
		}
		return keys[i].cached < keys[j].cached
	})
	return keys
}

func sortedChannelSweepKeys(m map[channelSweepMetric]uint64) []channelSweepMetric {
	keys := make([]channelSweepMetric, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].task != keys[j].task {
			return keys[i].task < keys[j].task
		}
		return keys[i].result < keys[j].result
	})
	return keys
}

func sortedChannelFailureKeys(m map[channelFailureMetric]uint64) []channelFailureMetric {
	keys := make([]channelFailureMetric, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].task != keys[j].task {
			return keys[i].task < keys[j].task
		}
		return keys[i].code < keys[j].code
	})
	return keys
}

func sortedMemoryFindKeys(m map[memoryFindMetric]memoryFindCounters) []memoryFindMetric {
	keys := make([]memoryFindMetric, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].result != keys[j].result {
			return keys[i].result < keys[j].result
		}
		return keys[i].omitted < keys[j].omitted
	})
	return keys
}

func label(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
