package admin

// CorpusSearchQueries lets a test ask the planner about the searches an erasure runs.
// ExpiredReplaysQuery lets a test ask the planner about what retention follows.
const ExpiredReplaysQuery = expiredReplaysSQL

func CorpusSearchQueries() map[string]string {
	return map[string]string{"copies": copySearchSQL, "replays": replaySearchSQL}
}

// BetweenToolReads runs f after Tools has read the published tools and before
// it reads where their servers are stored: a test changes the store there.
func (c *Curator) BetweenToolReads(f func()) { c.betweenToolReads = f }
