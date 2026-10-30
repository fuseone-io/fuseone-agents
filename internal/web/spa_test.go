package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// The build writes a .br and a .gz beside each compressible file. The bytes
// here are markers, not real compressed data: the handler never decodes what
// it serves, and distinct markers show which of the three files was sent.
func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html>")},
		"index.html.br":        {Data: []byte("BR:index")},
		"index.html.gz":        {Data: []byte("GZ:index")},
		"assets/app-abc.js":    {Data: []byte("console.log(1)")},
		"assets/app-abc.js.br": {Data: []byte("BR:app-abc.js")},
		"assets/app-abc.js.gz": {Data: []byte("GZ:app-abc.js")},
		"assets/app-abc.css":   {Data: []byte("body{}")},
		"assets/font.woff2":    {Data: []byte("wOF2")},
	}
}

func serve(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	return serveAccepting(t, target, "")
}

func serveAccepting(t *testing.T, target, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	spaHandler(http.FS(testFS())).ServeHTTP(rec, req)
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

func TestSPA_assetWithBrotliAccepted_servesTheBrotliVariant(t *testing.T) {
	t.Parallel()

	rec := serveAccepting(t, "/assets/app-abc.js", "gzip, deflate, br")
	if got := rec.Body.String(); got != "BR:app-abc.js" {
		t.Fatalf("body = %q, want the precompressed .br bytes", got)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Errorf("Content-Encoding = %q, want br", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding so a shared cache keys on it", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Errorf("Content-Type = %q, want the original file's type, not the .br's", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the immutable policy on a compressed asset too", got)
	}
}

func TestSPA_assetNegotiation_picksTheVariantTheClientAccepts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, accept, body, encoding string
	}{
		{"no header", "", "console.log(1)", ""},
		{"gzip only", "gzip", "GZ:app-abc.js", "gzip"},
		{"brotli refused by q=0", "br;q=0, gzip", "GZ:app-abc.js", "gzip"},
		{"both refused", "br;q=0, gzip;q=0", "console.log(1)", ""},
		{"identity only", "identity", "console.log(1)", ""},
		{"case and spacing", " GZIP ; q=0.5 ", "GZ:app-abc.js", "gzip"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := serveAccepting(t, "/assets/app-abc.js", tc.accept)
			if got := rec.Body.String(); got != tc.body {
				t.Errorf("body = %q, want %q", got, tc.body)
			}
			if got := rec.Header().Get("Content-Encoding"); got != tc.encoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tc.encoding)
			}
			if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("Vary = %q, want Accept-Encoding even on the plain answer", got)
			}
		})
	}
}

func TestSPA_assetWithoutVariants_servedPlainWhateverTheClientAccepts(t *testing.T) {
	t.Parallel()

	// Fonts are already compressed, so the build leaves them alone.
	rec := serveAccepting(t, "/assets/font.woff2", "br")
	if got := rec.Body.String(); got != "wOF2" {
		t.Errorf("body = %q, want the file itself", got)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none for a file with no variant", got)
	}
}

func TestSPA_appShell_isCompressedButNeverCachedImmutably(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"/", "/runs/run_a4d76"} {
		rec := serveAccepting(t, target, "br")
		if got := rec.Body.String(); got != "BR:index" {
			t.Errorf("%s: body = %q, want the compressed app shell", target, got)
		}
		if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q, want text/html", target, got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache so a deploy is picked up", target, got)
		}
	}
}
