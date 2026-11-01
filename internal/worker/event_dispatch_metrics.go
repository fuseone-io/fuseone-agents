package worker

import (
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
eventDispatchGauges is the last observation of the event dispatch backlog
(#228), as this worker read it.

Gauges of a periodic query, not a durable store: Prometheus keeps the
history, and the health page asks the database itself. The only label is the
backlog state, from a two-word vocabulary; no run, agent or transaction id
can reach a sample because the observation carries none.
*/
type eventDispatchGauges struct {
	mu   sync.Mutex
	last eventDispatchReading
}

// eventDispatchReading is what one scrape renders, copied out of the lock.
type eventDispatchReading struct {
	observed bool
	backlog  domain.EventDispatchBacklog
	at       time.Time
	failures uint64
}

// Backlog states: the only label values the backlog gauge takes.
const (
	backlogReady      = "ready"
	backlogHeldByXmin = "held_by_xmin"
)

// EventDispatch records one observation of the backlog, read at now.
func (r *MetricsRegistry) EventDispatch(b domain.EventDispatchBacklog, now time.Time) {
	if r == nil {
		return
	}
	g := &r.eventDispatch
	g.mu.Lock()
	defer g.mu.Unlock()
	g.last.observed, g.last.backlog, g.last.at = true, b, now
}

// EventDispatchUnobserved counts an observation that could not be read. The
// gauges keep the last good one; the timestamp says how old it is.
func (r *MetricsRegistry) EventDispatchUnobserved() {
	if r == nil {
		return
	}
	g := &r.eventDispatch
	g.mu.Lock()
	defer g.mu.Unlock()
	g.last.failures++
}

func (g *eventDispatchGauges) snapshot() eventDispatchReading {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.last
}

func renderEventDispatchMetrics(w io.Writer, g eventDispatchReading) {
	fmt.Fprintln(w, "# HELP fuseone_event_dispatch_observe_failures_total Event dispatch backlog reads that failed.")
	fmt.Fprintln(w, "# TYPE fuseone_event_dispatch_observe_failures_total counter")
	fmt.Fprintf(w, "fuseone_event_dispatch_observe_failures_total %d\n", g.failures)
	if !g.observed {
		return
	}
	b, now := g.backlog, g.at
	fmt.Fprintln(w, "# HELP fuseone_event_dispatch_backlog_runs Finished runs past the event dispatch cursor, by whether the feed can read them now or xmin holds them.")
	fmt.Fprintln(w, "# TYPE fuseone_event_dispatch_backlog_runs gauge")
	fmt.Fprintf(w, "fuseone_event_dispatch_backlog_runs{state=%s} %d\n", label(backlogReady), b.Ready)
	fmt.Fprintf(w, "fuseone_event_dispatch_backlog_runs{state=%s} %d\n", label(backlogHeldByXmin), b.HeldByXmin)
	gauge(w, "fuseone_event_dispatch_backlog_capped",
		"1 when a backlog count stopped at its cap: the true count is at least that.", boolGauge(b.Capped))
	gauge(w, "fuseone_event_dispatch_lag_seconds",
		"How long the oldest finished run past the cursor has waited.", seconds(b.Lag(now)))
	gauge(w, "fuseone_event_dispatch_held_by_xmin",
		"1 when committed finished runs wait on the oldest open transaction.", boolGauge(b.Held()))
	renderXactAge(w, b, now)
	gauge(w, "fuseone_event_dispatch_degraded",
		"1 when the event dispatch lag is past its threshold.", boolGauge(b.Status(now) == domain.EventDispatchDegraded))
	gauge(w, "fuseone_event_dispatch_observed_timestamp_seconds",
		"When this worker last read the event dispatch backlog.", float64(now.Unix()))
}

// renderXactAge omits the age when the role cannot see it: a zero would read
// as nothing open.
func renderXactAge(w io.Writer, b domain.EventDispatchBacklog, now time.Time) {
	age, known := b.XactAge(now)
	gauge(w, "fuseone_event_dispatch_oldest_xact_age_known",
		"0 when a transaction is open and the database role may not see when it began.", boolGauge(known))
	fmt.Fprintln(w, "# HELP fuseone_event_dispatch_oldest_xact_age_seconds Age of the oldest open transaction holding an id, which sets xmin.")
	fmt.Fprintln(w, "# TYPE fuseone_event_dispatch_oldest_xact_age_seconds gauge")
	if known {
		fmt.Fprintf(w, "fuseone_event_dispatch_oldest_xact_age_seconds %s\n", formatGauge(seconds(age)))
	}
}

func gauge(w io.Writer, name, help string, value float64) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %s\n", name, help, name, name, formatGauge(value))
}

func formatGauge(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func seconds(d time.Duration) float64 { return d.Seconds() }

func boolGauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
