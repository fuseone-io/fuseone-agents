package httpapi

import (
	"time"

	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

// unparseableSchedule is the first schedule this platform cannot read, or "".
//
// Asked before the version is written. A schedule that reached the table
// unparseable would be read as due forever; one that never reached it at all
// is an agent the screen calls scheduled and no clock has heard of.
func unparseableSchedule(s spec.Spec, now time.Time) string {
	for _, schedule := range spec.CronSchedules(s) {
		if _, err := trigger.NextAfter(schedule, now); err != nil {
			return schedule
		}
	}
	return ""
}
