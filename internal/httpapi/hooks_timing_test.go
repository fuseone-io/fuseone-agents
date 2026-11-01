package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// drained records whether the handler read the body to its end.
type drained struct {
	r    io.Reader
	done bool
}

func (d *drained) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)
	if err == io.EOF {
		d.done = true
	}
	return n, err
}

/*
The 401 takes as long for every path, whatever kind it is.

A GitHub path has to read the body before it can check a signature. If an
unknown path, or an ordinary one with the wrong key, answered before reading
it, a client sending a slow body would learn which paths come from GitHub by
how long the refusal took — the map of the installation the uniform 401 is
there to hide. So every request reads its bounded body before any decision
that differs by kind.
*/
func TestHook_anyRefusal_isAnsweredOnlyAfterTheBodyIsRead(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ path, secret string }{
		"a path nobody declared":         {"gh/none", ""},
		"an ordinary path, wrong key":    {"crm/ticket", "guess"},
		"an ordinary path, no key":       {"crm/ticket", ""},
		"a GitHub path, wrong signature": {"gh/reviewer", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mux, _, _ := ghServer(t)
			body := &drained{r: strings.NewReader(`{"pad":"` + strings.Repeat("x", 64<<10) + `"}`)}
			req := httptest.NewRequest(http.MethodPost, "/hooks/"+c.path, body)
			if c.secret != "" {
				req.Header.Set("X-FuseOne-Secret", c.secret)
			}
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if !body.done {
				t.Error("refused before reading the body: the time to a 401 tells the path's kind")
			}
		})
	}
}
