package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/fuseone/agents/internal/testdb"
)

/*
A classification level is said or left out, never null (NT-019).

Decoded into Go, a level left out and a level sent as null are both nil, so a
handler cannot tell them apart. Only one of them means the platform's ceiling;
taken as the other, a client's null would write a ruling stronger than the
owner's. Through the real route and its real decoding, null is refused and
nothing is stored, while a level left out still writes the platform's.
*/
func TestClassifyJourney_aNullLevel_isRefusedAndStoresNothing(t *testing.T) {
	pool := testdb.Pool(t)
	w := readBackWorldOn(t, pool)
	const tool = "classify-null.lookup"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.WithoutCancel(t.Context()),
			`delete from settings where kind = 'tool_classification' and name = $1`, tool)
	})
	path := "/admin/tools/" + tool + "/classification"

	if code, answer := w.send(t, http.MethodPut, path, map[string]any{"level": nil, "effect": "read"}); code != http.StatusBadRequest {
		t.Errorf("level null: %d %s, want 400", code, answer)
	}
	if n := rulingsOf(t, tool); n != 0 {
		t.Fatalf("%d rulings stored from a null level, want none", n)
	}
	if code, answer := w.send(t, http.MethodPut, path, map[string]any{"effect": "read"}); code >= 300 {
		t.Errorf("level left out: %d %s, want the platform's ruling written", code, answer)
	}
	if n := rulingsOf(t, tool); n != 1 {
		t.Errorf("%d rulings after a level left out, want the platform's one", n)
	}
}

func rulingsOf(t *testing.T, tool string) int {
	t.Helper()
	var n int
	if err := testdb.Pool(t).QueryRow(context.Background(),
		`select count(*) from settings where kind = 'tool_classification' and name = $1`, tool).Scan(&n); err != nil {
		t.Fatalf("count rulings: %v", err)
	}
	return n
}
