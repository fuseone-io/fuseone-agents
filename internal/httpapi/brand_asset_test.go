package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// fakeAssets enforces what the store does — the same content check, the same
// addressing — because a fake that accepted anything would let this suite
// pass over a handler that skipped the check.
type fakeAssets struct{ stored map[string]admin.BrandAsset }

func (f *fakeAssets) Put(
	_ context.Context, _ domain.UserID, _ domain.Scope, body []byte,
) (admin.BrandAsset, error) {
	contentType, err := admin.CheckBrandAsset(body)
	if err != nil {
		return admin.BrandAsset{}, err
	}
	sum := sha256.Sum256(body)
	asset := admin.BrandAsset{SHA256: hex.EncodeToString(sum[:]), ContentType: contentType, Body: body}
	if f.stored == nil {
		f.stored = map[string]admin.BrandAsset{}
	}
	f.stored[asset.SHA256] = asset
	return asset, nil
}

func (f *fakeAssets) Get(_ context.Context, sha string) (admin.BrandAsset, error) {
	asset, ok := f.stored[sha]
	if !ok {
		return admin.BrandAsset{}, admin.ErrBrandAssetNotFound
	}
	return asset, nil
}

var pngBody = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR-logo")

func upload(t *testing.T, store *fakeAssets, ctx context.Context, body []byte) openapi.UploadBrandAssetResponseObject {
	t.Helper()
	srv := NewServer(ledger.NewMemory(), "test").WithBrandAssets(store)
	resp, err := srv.UploadBrandAsset(ctx, openapi.UploadBrandAssetRequestObject{Body: bytes.NewReader(body)})
	if err != nil {
		t.Fatalf("UploadBrandAsset: %v", err)
	}
	return resp
}

func TestUploadBrandAsset_requiresBrandWrite(t *testing.T) {
	t.Parallel()
	store := &fakeAssets{}

	resp := upload(t, store, as(domain.RoleApprover), pngBody)

	if _, ok := resp.(openapi.UploadBrandAsset403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want forbidden", resp)
	}
	if len(store.stored) != 0 {
		t.Fatalf("refused upload still stored %d assets", len(store.stored))
	}
}

func TestUploadBrandAsset_returnsTheHashAndTheInternalURL(t *testing.T) {
	t.Parallel()

	resp := upload(t, &fakeAssets{}, as(domain.RoleCurator), pngBody)

	got, ok := resp.(openapi.UploadBrandAsset201JSONResponse)
	sum := sha256.Sum256(pngBody)
	want := hex.EncodeToString(sum[:])
	if !ok || got.Sha256 != want || got.Url != "/branding/assets/"+want || got.ContentType != "image/png" {
		t.Fatalf("response = %#v, want 201 naming %s", resp, want)
	}
}

func TestUploadBrandAsset_contentDecidesNotTheName(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"text called logo.png": "just some text, saved as logo.png",
		"svg":                  `<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`,
	} {
		resp := upload(t, &fakeAssets{}, as(domain.RoleCurator), []byte(body))
		got, ok := resp.(openapi.UploadBrandAsset400ApplicationProblemPlusJSONResponse)
		if !ok {
			t.Fatalf("%s: response = %T, want bad request", name, resp)
		}
		if name == "svg" && !strings.Contains(*got.Detail, "SVG") {
			t.Errorf("svg refusal does not say why: %q", *got.Detail)
		}
	}
}

func TestUploadBrandAsset_overTheLimit_payloadTooLarge(t *testing.T) {
	t.Parallel()
	body := append(append([]byte{}, pngBody...), make([]byte, admin.MaxBrandAssetBytes)...)

	resp := upload(t, &fakeAssets{}, as(domain.RoleCurator), body)

	if _, ok := resp.(openapi.UploadBrandAsset413ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want 413", resp)
	}
}

// serveAsset asks the public route with no principal at all, which is how
// the sign-in screen asks.
func serveAsset(store *fakeAssets, path string, header http.Header) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	NewBrandAssetRoute(store).Mount(mux)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func storedPNG(t *testing.T) (*fakeAssets, string) {
	t.Helper()
	store := &fakeAssets{}
	asset, err := store.Put(context.Background(), "usr_ana", domain.Scope{}, pngBody)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	return store, asset.SHA256
}

func TestBrandAssetRoute_servesAnonymouslyWithImmutableCaching(t *testing.T) {
	t.Parallel()
	store, sha := storedPNG(t)

	rec := serveAsset(store, "/branding/assets/"+sha, nil)

	want := map[string]string{
		"Content-Type":           "image/png",
		"ETag":                   `"` + sha + `"`,
		"Cache-Control":          "public, max-age=31536000, immutable",
		"X-Content-Type-Options": "nosniff",
	}
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), pngBody) {
		t.Fatalf("status = %d, body %d bytes", rec.Code, rec.Body.Len())
	}
	for name, value := range want {
		if got := rec.Header().Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

func TestBrandAssetRoute_matchingETag_notModified(t *testing.T) {
	t.Parallel()
	store, sha := storedPNG(t)

	rec := serveAsset(store, "/branding/assets/"+sha, http.Header{"If-None-Match": {`"` + sha + `"`}})

	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("status = %d, body %d bytes; want 304 and nothing", rec.Code, rec.Body.Len())
	}
	if rec.Header().Get("ETag") != `"`+sha+`"` {
		t.Errorf("304 without the ETag: %q", rec.Header().Get("ETag"))
	}
}

func TestBrandAssetRoute_staleETag_servesTheImage(t *testing.T) {
	t.Parallel()
	store, sha := storedPNG(t)

	rec := serveAsset(store, "/branding/assets/"+sha, http.Header{"If-None-Match": {`"` + strings.Repeat("0", 64) + `"`}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestBrandAssetRoute_unknownOrMalformed_notFound(t *testing.T) {
	t.Parallel()
	store, _ := storedPNG(t)

	for _, path := range []string{
		"/branding/assets/" + strings.Repeat("a", 64),
		"/branding/assets/not-a-hash",
	} {
		if rec := serveAsset(store, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
	}
}
