package main

import (
	"net/http"
	"testing"
)

/*
A channel's answer carries its conversations and the identities bound on it, so its revision
names them too. A strong validator that held still while the body changed would let a client
holding the older answer remove a connection, and with it a conversation it never saw.
*/
func TestChannelETag_whatIsAttachedMovesIt(t *testing.T) {
	w := newReadBackWorld(t)
	r := managedResources()[2]
	stored(t, w, r, "ETAG-SECRET-ONE")
	t.Cleanup(func() { w.call(t, etagCall{method: http.MethodDelete, path: r.path + "/identities/U-ETAG"}) })
	read := func() string {
		t.Helper()
		got := w.call(t, etagCall{method: http.MethodGet, path: r.path})
		if got.Code != http.StatusOK {
			t.Fatalf("GET = %d: %s", got.Code, got.Body)
		}
		return got.Header().Get("ETag")
	}
	bare := read()

	w.put(t, r.path+"/conversations/C-ETAG", map[string]any{"company": "default", "area": "platform"})
	withRoom := read()
	if withRoom == bare {
		t.Fatal("attaching a conversation did not move the revision")
	}
	w.put(t, r.path+"/identities/U-ETAG", map[string]any{"principal": "usr_ana"})
	withIdentity := read()
	if withIdentity == withRoom {
		t.Fatal("binding an identity did not move the revision")
	}

	changed := r.body("")
	r.change(changed)
	if put := w.call(t, etagCall{method: http.MethodPut, path: r.path, ifMatch: []string{bare}, body: changed}); put.Code != http.StatusPreconditionFailed {
		t.Errorf("a write over the answer before the conversation = %d, want 412", put.Code)
	}
	if del := w.call(t, etagCall{method: http.MethodDelete, path: r.path, ifMatch: []string{bare}}); del.Code != http.StatusPreconditionFailed {
		t.Fatalf("a removal over the answer before the conversation = %d, want 412", del.Code)
	}

	// What a write answers is what the next read does.
	put := w.call(t, etagCall{method: http.MethodPut, path: r.path, ifMatch: []string{withIdentity}, body: changed})
	if put.Code != http.StatusNoContent || put.Header().Get("ETag") != read() {
		t.Fatalf("PUT = %d with %q, then GET answered %q", put.Code, put.Header().Get("ETag"), read())
	}
}
