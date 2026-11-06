package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A store's fence holds connections outside the pool, up to FenceSlots. That budget
// is per process only while each process builds one store, so a second one is
// connections nobody counted. One here for serve and one for the worker.
func TestRegressionStore_isBuiltOncePerProcess(t *testing.T) {
	t.Parallel()

	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	built := map[string]int{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s: %v", source, err)
		}
		if n := strings.Count(string(raw), "regression.NewStore("); n > 0 {
			built[source] = n
		}
	}
	if built["serve.go"] != 1 || built["worker_loops.go"] != 1 || len(built) != 2 {
		t.Fatalf("regression.NewStore is built in %v, want once in serve.go and once in worker_loops.go", built)
	}
}
