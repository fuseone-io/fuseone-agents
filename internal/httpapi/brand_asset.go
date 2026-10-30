package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// BrandAssets is where uploaded logos and icons are kept.
type BrandAssets interface {
	Put(ctx context.Context, by domain.UserID, scope domain.Scope, body []byte) (admin.BrandAsset, error)
	Get(ctx context.Context, sha string) (admin.BrandAsset, error)
}

// BrandAssetReader is the one question the public route may ask.
type BrandAssetReader interface {
	Get(ctx context.Context, sha string) (admin.BrandAsset, error)
}

func (s *Server) WithBrandAssets(assets BrandAssets) *Server {
	s.brandAssets = assets
	return s
}

// UploadBrandAsset stores a logo or icon, with the same authority as
// changing the brand. It never changes the brand itself.
func (s *Server) UploadBrandAsset(
	ctx context.Context, req openapi.UploadBrandAssetRequestObject,
) (openapi.UploadBrandAssetResponseObject, error) {
	if refused := s.mayWriteBrand(ctx); refused != nil {
		return openapi.UploadBrandAsset403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.brandAssets == nil || req.Body == nil {
		return openapi.UploadBrandAsset400ApplicationProblemPlusJSONResponse(
			notStored("this installation has no store for branding images")), nil
	}
	// One byte past the limit is enough to know it is over, and reading no
	// further keeps an oversized upload from being held in memory whole.
	body, err := io.ReadAll(io.LimitReader(req.Body, admin.MaxBrandAssetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read brand asset: %w", err)
	}
	asset, err := s.brandAssets.Put(ctx, callerOf(ctx), adminScope, body)
	switch {
	case errors.Is(err, admin.ErrBrandAssetTooLarge):
		return openapi.UploadBrandAsset413ApplicationProblemPlusJSONResponse(refusal(
			http.StatusRequestEntityTooLarge, CodeInvalidInput, "Image too large",
			fmt.Sprintf("a branding image is at most %d KiB", admin.MaxBrandAssetBytes>>10))), nil
	case errors.Is(err, admin.ErrBrandAssetRefused):
		return openapi.UploadBrandAsset400ApplicationProblemPlusJSONResponse(notStored(err.Error())), nil
	case err != nil:
		return nil, fmt.Errorf("store brand asset: %w", err)
	}
	return openapi.UploadBrandAsset201JSONResponse{
		Sha256:      asset.SHA256,
		Url:         admin.BrandAssetPath(asset.SHA256),
		ContentType: openapi.BrandAssetContentType(asset.ContentType),
	}, nil
}

// BrandAssetRoute serves uploaded branding images. Unauthenticated on
// purpose, like /auth/providers: the sign-in screen shows the brand before
// there is anybody to authenticate.
type BrandAssetRoute struct{ assets BrandAssetReader }

func NewBrandAssetRoute(assets BrandAssetReader) *BrandAssetRoute {
	return &BrandAssetRoute{assets: assets}
}

func (b *BrandAssetRoute) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /branding/assets/{sha256}", b.serve)
}

/*
serve answers with the bytes and the type decided at upload, never one sniffed
again here or by the browser (nosniff). The name is the hash of the content,
so the bytes behind a URL can never change: the ETag is the hash and the
response may be cached for a year.
*/
func (b *BrandAssetRoute) serve(w http.ResponseWriter, r *http.Request) {
	sha := r.PathValue("sha256")
	if !admin.ValidBrandAssetHash(sha) {
		http.NotFound(w, r)
		return
	}
	asset, err := b.assets.Get(r.Context(), sha)
	if errors.Is(err, admin.ErrBrandAssetNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("serve brand asset", "sha256", sha, "err", err)
		http.Error(w, "branding image unavailable", http.StatusInternalServerError)
		return
	}

	etag := `"` + asset.SHA256 + `"`
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", asset.ContentType)
	h.Set("Content-Length", strconv.Itoa(len(asset.Body)))
	_, _ = w.Write(asset.Body)
}

// etagMatches is If-None-Match's weak comparison (RFC 9110 §13.1.2): a list
// of tags, any of which may carry W/, or a bare "*".
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}
