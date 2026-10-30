package admin_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

var (
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	jpegMagic = []byte{0xFF, 0xD8, 0xFF, 0xE0}
)

// image is a body whose first bytes are the format's signature and whose
// remainder makes each one distinct, so two calls never share a hash.
func image(magic []byte, tag string) []byte {
	return append(append([]byte{}, magic...), []byte("\x00\x00\x00\rIHDR-"+tag)...)
}

func webp(tag string) []byte {
	return append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), []byte(tag)...)
}

func TestCheckBrandAsset_acceptsTheThreeRasterFormatsByContent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		body []byte
		want string
	}{
		{"png", image(pngMagic, "a"), "image/png"},
		{"jpeg", image(jpegMagic, "a"), "image/jpeg"},
		{"webp", webp("a"), "image/webp"},
	} {
		got, err := admin.CheckBrandAsset(tc.body)
		if err != nil || got != tc.want {
			t.Errorf("%s: CheckBrandAsset = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
}

func TestCheckBrandAsset_textNamedAsAPNG_refused(t *testing.T) {
	t.Parallel()

	// What a browser would send for "logo.png" holding a note. Nothing about
	// the file's name or the declared type reaches the check — only bytes.
	_, err := admin.CheckBrandAsset([]byte("this is a text file called logo.png\n"))
	if !errors.Is(err, admin.ErrBrandAssetRefused) {
		t.Fatalf("err = %v, want refused", err)
	}
}

func TestCheckBrandAsset_svg_refusedAndSaysWhy(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"\uFEFF  <?xml version=\"1.0\"?>\n<svg xmlns=\"http://www.w3.org/2000/svg\"/>",
	} {
		_, err := admin.CheckBrandAsset([]byte(body))
		if !errors.Is(err, admin.ErrBrandAssetRefused) || !strings.Contains(err.Error(), "SVG") {
			t.Fatalf("CheckBrandAsset(%.20q) = %v, want an SVG refusal", body, err)
		}
	}
}

func TestCheckBrandAsset_overTheLimit_refused(t *testing.T) {
	t.Parallel()

	at := append(image(pngMagic, "x"), make([]byte, admin.MaxBrandAssetBytes)...)[:admin.MaxBrandAssetBytes]
	if _, err := admin.CheckBrandAsset(at); err != nil {
		t.Fatalf("at the limit: %v, want accepted", err)
	}
	over := append(at, 0)
	if _, err := admin.CheckBrandAsset(over); !errors.Is(err, admin.ErrBrandAssetTooLarge) {
		t.Fatalf("one byte over: %v, want too large", err)
	}
}

func newAssets(t *testing.T) (*admin.Branding, *admin.BrandAssets, *pgxpool.Pool) {
	t.Helper()
	branding, pool := newBranding(t)
	return branding, admin.NewBrandAssets(pool), pool
}

func put(t *testing.T, assets *admin.BrandAssets, body []byte) admin.BrandAsset {
	t.Helper()
	got, err := assets.Put(context.Background(), "usr_ana", domain.Scope{}, body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	return got
}

func TestBrandAssets_putIsAddressedByContentAndDedupes(t *testing.T) {
	_, assets, pool := newAssets(t)
	body := image(pngMagic, "dedupe")
	sum := sha256.Sum256(body)

	first, second := put(t, assets, body), put(t, assets, body)
	if first.SHA256 != hex.EncodeToString(sum[:]) || second.SHA256 != first.SHA256 {
		t.Fatalf("hashes = %s, %s", first.SHA256, second.SHA256)
	}
	got, err := assets.Get(context.Background(), first.SHA256)
	if err != nil || !bytes.Equal(got.Body, body) || got.ContentType != "image/png" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	var rows int
	if err := pool.QueryRow(context.Background(),
		`select count(*) from branding_assets where sha256 = $1`, first.SHA256).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows = %d, %v; want one", rows, err)
	}
}

func TestBrandAssets_getUnknown_notFound(t *testing.T) {
	_, assets, _ := newAssets(t)

	_, err := assets.Get(context.Background(), strings.Repeat("a", 64))
	if !errors.Is(err, admin.ErrBrandAssetNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestBranding_referenceToAnAssetNeverUploaded_refused(t *testing.T) {
	store, _, _ := newAssets(t)

	err := store.Set(context.Background(), "usr_ana", domain.Scope{}, admin.BrandingConfig{
		DisplayName: "Acme", LogoURL: admin.BrandAssetPath(strings.Repeat("b", 64)),
	})
	if !errors.Is(err, admin.ErrBrandingInvalid) {
		t.Fatalf("err = %v, want invalid branding", err)
	}
}

func set(t *testing.T, store *admin.Branding, logo, icon string) {
	t.Helper()
	if err := store.Set(context.Background(), "usr_ana", domain.Scope{}, admin.BrandingConfig{
		DisplayName: "Acme", LogoURL: logo, IconURL: icon,
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
}

func exists(t *testing.T, assets *admin.BrandAssets, sha string) bool {
	t.Helper()
	_, err := assets.Get(context.Background(), sha)
	if err != nil && !errors.Is(err, admin.ErrBrandAssetNotFound) {
		t.Fatalf("Get: %v", err)
	}
	return err == nil
}

func TestBranding_replacingTheLogo_deletesTheAssetNothingReferences(t *testing.T) {
	store, assets, _ := newAssets(t)
	old, next := put(t, assets, image(pngMagic, "old")), put(t, assets, image(pngMagic, "new"))

	set(t, store, admin.BrandAssetPath(old.SHA256), "")
	set(t, store, admin.BrandAssetPath(next.SHA256), "")

	if exists(t, assets, old.SHA256) {
		t.Fatal("the replaced logo is still stored")
	}
	if !exists(t, assets, next.SHA256) {
		t.Fatal("the logo in force was deleted")
	}
}

func TestBranding_replacingTheLogo_keepsAnAssetTheIconStillUses(t *testing.T) {
	store, assets, _ := newAssets(t)
	shared, next := put(t, assets, image(pngMagic, "shared")), put(t, assets, image(pngMagic, "next"))

	// The same upload serves as logo and icon; changing the logo alone must
	// not take the icon's image with it.
	set(t, store, admin.BrandAssetPath(shared.SHA256), admin.BrandAssetPath(shared.SHA256))
	set(t, store, admin.BrandAssetPath(next.SHA256), admin.BrandAssetPath(shared.SHA256))

	if !exists(t, assets, shared.SHA256) {
		t.Fatal("an asset the icon still references was deleted")
	}
}

func TestBranding_trailRecordsTheHashAndNeverTheBytes(t *testing.T) {
	store, assets, pool := newAssets(t)
	body := image(pngMagic, "audited-"+strings.Repeat("z", 64))
	asset := put(t, assets, body)

	set(t, store, admin.BrandAssetPath(asset.SHA256), "")

	for _, target := range []string{"installation", asset.SHA256} {
		_, detail := lastTrailDetail(t, pool, target)
		if !strings.Contains(detail, asset.SHA256) && target == "installation" {
			t.Errorf("%s: detail %s does not name the hash", target, detail)
		}
		for _, leak := range []string{base64.StdEncoding.EncodeToString(body), "audited-zzzz", "IHDR"} {
			if strings.Contains(detail, leak) {
				t.Errorf("%s: detail carries image bytes: %s", target, detail)
			}
		}
	}
}
