package worker

import (
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/domain"
)

type runtimeCounters struct {
	promptToolResultBytes       uint64
	promptToolResultElidedBytes uint64
	cacheReadTokens             uint64
	cacheWriteTokens            uint64
	canonicalDuplicates         uint64
	investigationStalls         uint64
}

// Planning records aggregate wire composition and provider cache behavior.
// Tool-level attribution remains in the ledger; Prometheus stays bounded.
func (m *PoolMetrics) Planning(prompt domain.PromptInputBreakdown, cost domain.Cost) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime.promptToolResultBytes += positive(prompt.ToolResults)
	m.runtime.promptToolResultElidedBytes += positive(prompt.ToolResultsElided)
	m.runtime.cacheReadTokens += positive(cost.CacheReadTokens)
	m.runtime.cacheWriteTokens += positive(cost.CacheWriteTokens)
}

func (m *PoolMetrics) CanonicalDuplicate() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime.canonicalDuplicates++
}

func (m *PoolMetrics) InvestigationStalled() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime.investigationStalls++
}

func positive(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func renderRuntimeEfficiencyMetrics(w http.ResponseWriter, snap []poolSnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_worker_prompt_tool_result_bytes_total Tool-result bytes sent to or elided from model prompts.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_prompt_tool_result_bytes_total counter")
	for _, p := range snap {
		fmt.Fprintf(w, "fuseone_worker_prompt_tool_result_bytes_total{pool=%s,disposition=%s} %d\n",
			label(p.pool), label("sent"), p.runtime.promptToolResultBytes)
		fmt.Fprintf(w, "fuseone_worker_prompt_tool_result_bytes_total{pool=%s,disposition=%s} %d\n",
			label(p.pool), label("elided"), p.runtime.promptToolResultElidedBytes)
	}
	renderCacheTokenMetrics(w, snap)
	renderRuntimeEventMetrics(w, snap)
}

func renderCacheTokenMetrics(w http.ResponseWriter, snap []poolSnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_worker_model_cache_tokens_total Provider prompt-cache tokens by operation.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_model_cache_tokens_total counter")
	for _, p := range snap {
		fmt.Fprintf(w, "fuseone_worker_model_cache_tokens_total{pool=%s,operation=%s} %d\n",
			label(p.pool), label("read"), p.runtime.cacheReadTokens)
		fmt.Fprintf(w, "fuseone_worker_model_cache_tokens_total{pool=%s,operation=%s} %d\n",
			label(p.pool), label("write"), p.runtime.cacheWriteTokens)
	}
}

func renderRuntimeEventMetrics(w http.ResponseWriter, snap []poolSnapshot) {
	fmt.Fprintln(w, "# HELP fuseone_worker_canonical_duplicates_total Calls skipped by canonical per-run identity.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_canonical_duplicates_total counter")
	fmt.Fprintln(w, "# HELP fuseone_worker_investigation_stalls_total Runs parked after repeated reads returned no new evidence.")
	fmt.Fprintln(w, "# TYPE fuseone_worker_investigation_stalls_total counter")
	for _, p := range snap {
		fmt.Fprintf(w, "fuseone_worker_canonical_duplicates_total{pool=%s} %d\n",
			label(p.pool), p.runtime.canonicalDuplicates)
		fmt.Fprintf(w, "fuseone_worker_investigation_stalls_total{pool=%s} %d\n",
			label(p.pool), p.runtime.investigationStalls)
	}
}
