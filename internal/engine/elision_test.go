package engine

import "testing"

// What compaction removed has to be measured where it happens.
//
// The composition records what was sent; the raw stays in the content store.
// Subtracting one from the other on a screen would be arithmetic across two
// sources that were never meant to be compared, and the answer would drift the
// first time either changed. Measured here, the saving is a fact the run
// recorded rather than one a reader inferred.
func TestCompaction_recordsWhatItRemoved(t *testing.T) {
	t.Parallel()

	big := make([]byte, 200<<10)
	for i := range big {
		big[i] = 'a'
	}

	var elided int64
	out := compactToolResult("grafana.query_loki_logs", big, &elided)

	if len(out) >= len(big) {
		t.Fatalf("nothing was compacted: %d bytes out of %d", len(out), len(big))
	}
	if elided <= 0 {
		t.Fatal("compaction removed content and recorded none of it")
	}
	// The three parts account for the whole: what was sent, what was removed,
	// and the note wrapped around them.
	if int64(len(big))-elided > int64(len(out)) {
		t.Errorf("sent %d and elided %d, which does not add up to %d",
			len(out), elided, len(big))
	}
}

func TestCompaction_smallResultRecordsNoSaving(t *testing.T) {
	t.Parallel()

	var elided int64
	small := []byte("only a few bytes")
	if got := compactToolResult("grafana.query_loki_logs", small, &elided); string(got) != string(small) {
		t.Error("a small result was rewritten")
	}
	if elided != 0 {
		t.Errorf("elided = %d for a result that was not compacted", elided)
	}
}
