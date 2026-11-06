package admin

// CorpusSearchQueries lets a test ask the planner about the searches an erasure runs.
func CorpusSearchQueries() map[string]string {
	return map[string]string{"copies": copySearchSQL, "replays": replaySearchSQL}
}
