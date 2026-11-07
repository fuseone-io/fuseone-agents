package simulate

import "github.com/fuseone/agents/internal/domain"

/*
NotChecked names the cases in the corpus now that a pinned battery never ran.

Added after it, so they are neither broken nor held, and they do not stop a start by
themselves. The author still has to hear about them, because otherwise a corpus that grew
reads as covered. An erased case cannot run, and the report already calls it a gap. With no
pin there is nothing to say: read against the corpus as it is, a case that did not run has
already been called broken.
*/
func NotChecked(report Report, working []domain.RegressionCase) []string {
	if report.DatasetVersion == 0 {
		return nil
	}
	ran := make(map[string]bool, len(report.Cases))
	for _, c := range report.Cases {
		ran[c.ID] = true
	}
	var out []string
	for _, c := range working {
		if !ran[c.ID] && !c.Erased {
			out = append(out, c.ID)
		}
	}
	return out
}
