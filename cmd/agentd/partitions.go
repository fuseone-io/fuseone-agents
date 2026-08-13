package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/ledger"
)

// partitionSweep is how often the ledger's months are topped up.
//
// Daily, against twelve months of headroom: the interval only has to be
// shorter than the headroom, and by a wide margin, so an installation that is
// off for a long weekend or a long quarter comes back to somewhere to write.
const partitionSweep = 24 * time.Hour

// keepMonthsAhead makes the ledger partitions the coming year will need.
//
// It runs once at startup and then daily, and a failure is logged rather than
// fatal. A month that could not be created is not an outage: those steps land
// in the default partition and are recorded correctly. What is lost is the
// ability to archive that month later, which is worth a loud line in the log
// and not worth refusing to start.
func keepMonthsAhead(ctx context.Context, months *ledger.Partitions) {
	sweep := func() {
		if _, err := months.Ensure(ctx); err != nil {
			slog.Error("ledger partitions could not be created", "err", err)
			return
		}
		if stranded, err := months.Stranded(ctx); err == nil && stranded > 0 {
			slog.Warn("steps are in the ledger's default partition",
				"steps", stranded,
				"why", "a month arrived with no partition; those steps cannot be archived as one")
		}
	}

	sweep()
	ticker := time.NewTicker(partitionSweep)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
