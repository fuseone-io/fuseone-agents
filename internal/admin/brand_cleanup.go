package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/settings"
)

// assetRefs is every uploaded image a brand points at.
func assetRefs(config BrandingConfig) []string {
	var out []string
	for _, ref := range []string{config.LogoURL, config.IconURL} {
		if sha := brandAssetHash(ref); sha != "" {
			out = append(out, sha)
		}
	}
	return out
}

/*
lockBranding serialises every branding write, and with it every delete from
branding_assets, which only happens inside one.

Without it, under READ COMMITTED, a save replacing image X judges X
unreferenced while another save that names X is still uncommitted; that one
finds X present, both commit, and its brand points at a deleted image. Taken
before the stored brand is read, the second save judges only after the first
has committed. Uploads stay outside it: an insert never removes a row.
*/
func lockBranding(ctx context.Context, conn settings.DB) error {
	if _, err := conn.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended($1, 0))`, BrandingLock); err != nil {
		return fmt.Errorf("admin: lock branding: %w", err)
	}
	return nil
}

// requireAssets refuses a brand that points at an image nobody uploaded,
// inside the write's transaction: a broken image on the sign-in screen is
// found by the person who cannot sign in, not by the one who saved it.
func requireAssets(ctx context.Context, conn settings.DB, config BrandingConfig) error {
	for _, sha := range assetRefs(config) {
		var found bool
		if err := conn.QueryRow(ctx,
			`select exists(select 1 from branding_assets where sha256 = $1)`, sha,
		).Scan(&found); err != nil {
			return fmt.Errorf("admin: look up brand asset: %w", err)
		}
		if !found {
			return fmt.Errorf("%w: image %s was not uploaded to this installation", ErrBrandingInvalid, sha)
		}
	}
	return nil
}

/*
dropReplaced deletes the images the previous brand used and the new one does
not — but only those no branding setting references any more. It runs after
the new value is written, in the same transaction, so "still referenced" is
asked of what is about to be committed rather than of what was there before.

Hash addressing is what makes this safe: two settings pointing at the same
bytes point at the same row, so a reference survives exactly as long as some
setting holds it.
*/
func dropReplaced(
	ctx context.Context, conn settings.DB, store *settings.Store, previous BrandingConfig,
) error {
	stale := assetRefs(previous)
	if len(stale) == 0 {
		return nil
	}
	held, err := heldAssets(ctx, conn, store)
	if err != nil {
		return err
	}
	for _, sha := range stale {
		if held[sha] {
			continue
		}
		if _, err := conn.Exec(ctx, `delete from branding_assets where sha256 = $1`, sha); err != nil {
			return fmt.Errorf("admin: drop replaced brand asset: %w", err)
		}
	}
	return nil
}

func heldAssets(ctx context.Context, conn settings.DB, store *settings.Store) (map[string]bool, error) {
	all, err := store.ListTx(ctx, conn, settings.KindBranding)
	if err != nil {
		return nil, fmt.Errorf("admin: list branding: %w", err)
	}
	held := map[string]bool{}
	for _, set := range all {
		var config BrandingConfig
		if err := json.Unmarshal(set.Value, &config); err != nil {
			return nil, fmt.Errorf("admin: decode branding %s: %w", set.Name, err)
		}
		for _, sha := range assetRefs(config) {
			held[sha] = true
		}
	}
	return held, nil
}

// brandingDetail is what the trail keeps: that the brand changed, and which
// uploaded images it now names by hash. Never the bytes, and never an
// external URL, which can carry a tracking token.
func brandingDetail(config BrandingConfig) map[string]any {
	detail := map[string]any{
		"displayName":  config.DisplayName,
		"hasLogo":      config.LogoURL != "",
		"hasIcon":      config.IconURL != "",
		"primaryColor": config.PrimaryColor,
	}
	if sha := brandAssetHash(config.LogoURL); sha != "" {
		detail["logoAsset"] = sha
	}
	if sha := brandAssetHash(config.IconURL); sha != "" {
		detail["iconAsset"] = sha
	}
	return detail
}
