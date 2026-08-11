package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func serve(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()

	fsys := fstest.MapFS{
		"index.html":         {Data: []byte("<!doctype html>")},
		"assets/app-abc.js":  {Data: []byte("console.log(1)")},
		"assets/app-abc.css": {Data: []byte("body{}")},
	}
	rec := httptest.NewRecorder()
	spaHandler(http.FS(fsys)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestSPA_deepLink_servesTheAppSoTheBrowserRouterCanTakeOver(t *testing.T) {
	t.Parallel()

	rec := serve(t, "/runs/run_a4d76")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want the app for a route the router owns", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache so a deploy is picked up", got)
	}
}

func TestSPA_missingAsset_is404RatherThanTheAppShell(t *testing.T) {
	t.Parallel()

	// Asset names are content hashed, so a miss under /assets/ is never a
	// route — it is a client asking for a build that no longer exists.
	// Answering with index.html makes the browser parse HTML as JavaScript and
	// report a syntax error, which hides the real cause: a stale tab.
	rec := serve(t, "/assets/app-gone.js")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an asset that is not in this build", rec.Code)
	}
}

func TestSPA_hashedAsset_isCachedForever(t *testing.T) {
	t.Parallel()

	rec := serve(t, "/assets/app-abc.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want the asset", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the immutable policy", got)
	}
}
