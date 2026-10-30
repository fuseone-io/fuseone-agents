package admin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// MaxBrandAssetBytes is the largest logo or icon an installation accepts.
// The sign-in screen downloads it before anybody has signed in, so it is
// kept to what a mark needs, not what a photograph does.
const MaxBrandAssetBytes = 512 << 10

const brandAssetPrefix = "/branding/assets/"

// BrandingLock names the advisory lock every branding write takes before
// judging which images exist or are still referenced.
const BrandingLock = "branding:assets"

var (
	ErrBrandAssetRefused  = errors.New("admin: brand asset refused")
	ErrBrandAssetTooLarge = errors.New("admin: brand asset too large")
	ErrBrandAssetNotFound = errors.New("admin: brand asset not found")

	assetHash = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// BrandAsset is one uploaded image, named by the SHA-256 of its bytes.
type BrandAsset struct {
	SHA256      string
	ContentType string
	Body        []byte
}

// BrandAssets stores the images an installation serves itself, so an
// air-gapped one shows its brand without an outbound request.
type BrandAssets struct{ pool *pgxpool.Pool }

func NewBrandAssets(pool *pgxpool.Pool) *BrandAssets { return &BrandAssets{pool: pool} }

// BrandAssetPath is the reference branding stores and the console loads: a
// path on this installation, never the bytes.
func BrandAssetPath(sha string) string { return brandAssetPrefix + sha }

// ValidBrandAssetHash reports whether s can name an asset at all, so a route
// answers a malformed one without asking the database.
func ValidBrandAssetHash(s string) bool { return assetHash.MatchString(s) }

// brandAssetHash is the hash a branding reference points at, or "" for an
// external URL or a data image.
func brandAssetHash(ref string) string {
	sha, ok := strings.CutPrefix(ref, brandAssetPrefix)
	if !ok || !assetHash.MatchString(sha) {
		return ""
	}
	return sha
}

/*
CheckBrandAsset decides what an upload is from its bytes alone.

The file name and the Content-Type the browser sent are both whatever the
uploader says they are; the signature at the start of the file is what an image
decoder will actually act on. It returns the type the public route serves.

SVG is refused outright for now: it is a document that can carry script, and an
image the sign-in screen serves from this origin is the wrong place to find out
which browsers honour that.
*/
func CheckBrandAsset(body []byte) (string, error) {
	if len(body) > MaxBrandAssetBytes {
		return "", fmt.Errorf("%w: %d bytes, the limit is %d", ErrBrandAssetTooLarge, len(body), MaxBrandAssetBytes)
	}
	switch {
	case bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", nil
	case bytes.HasPrefix(body, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg", nil
	case len(body) >= 12 && bytes.Equal(body[:4], []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP")):
		return "image/webp", nil
	case looksLikeSVG(body):
		return "", fmt.Errorf("%w: SVG is not accepted for now because it can carry script; upload a PNG, JPEG or WebP",
			ErrBrandAssetRefused)
	}
	return "", fmt.Errorf("%w: the content is not a PNG, JPEG or WebP image", ErrBrandAssetRefused)
}

func looksLikeSVG(body []byte) bool {
	head := body[:min(len(body), 1024)]
	return bytes.Contains(bytes.ToLower(head), []byte("<svg"))
}

// Put stores an image after checking it, and records the upload by hash.
// The same bytes twice is the same row: the second Put changes nothing.
func (a *BrandAssets) Put(
	ctx context.Context, by domain.UserID, scope domain.Scope, body []byte,
) (BrandAsset, error) {
	contentType, err := CheckBrandAsset(body)
	if err != nil {
		return BrandAsset{}, err
	}
	sum := sha256.Sum256(body)
	asset := BrandAsset{SHA256: hex.EncodeToString(sum[:]), ContentType: contentType, Body: body}

	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return BrandAsset{}, fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		insert into branding_assets (sha256, content_type, body, created_by)
		values ($1, $2, $3, $4) on conflict (sha256) do nothing`,
		asset.SHA256, asset.ContentType, asset.Body, string(by)); err != nil {
		return BrandAsset{}, fmt.Errorf("admin: store brand asset: %w", err)
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: "branding.asset.uploaded", Target: asset.SHA256,
		Detail: map[string]any{"sha256": asset.SHA256, "contentType": contentType, "bytes": len(body)},
	}); err != nil {
		return BrandAsset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BrandAsset{}, fmt.Errorf("admin: commit brand asset: %w", err)
	}
	return asset, nil
}

// Get returns one stored image.
func (a *BrandAssets) Get(ctx context.Context, sha string) (BrandAsset, error) {
	if !ValidBrandAssetHash(sha) {
		return BrandAsset{}, ErrBrandAssetNotFound
	}
	asset := BrandAsset{SHA256: sha}
	err := a.pool.QueryRow(ctx,
		`select content_type, body from branding_assets where sha256 = $1`, sha,
	).Scan(&asset.ContentType, &asset.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return BrandAsset{}, ErrBrandAssetNotFound
	}
	if err != nil {
		return BrandAsset{}, fmt.Errorf("admin: read brand asset: %w", err)
	}
	return asset, nil
}
