package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

const brandingName = "installation"

var (
	ErrBrandingInvalid = errors.New("admin: invalid branding")
	hexColour          = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	// SVG data images are accepted for img src rendering only; revisit before using them as inline markup or CSS.
	dataImage = regexp.MustCompile(`^data:image/(png|jpeg|jpg|gif|webp|svg\+xml);base64,[A-Za-z0-9+/]+={0,2}$`)
)

// DefaultBranding is what an unconfigured installation presents.
var DefaultBranding = BrandingConfig{DisplayName: "FuseOne Agents"}

// BrandingConfig is the public identity of this installation.
type BrandingConfig struct {
	DisplayName  string `json:"displayName"`
	LogoURL      string `json:"logoUrl,omitempty"`
	IconURL      string `json:"iconUrl,omitempty"`
	PrimaryColor string `json:"primaryColor,omitempty"`
}

// Branding stores the installation's public identity.
type Branding struct {
	pool     *pgxpool.Pool
	settings *settings.Store
}

func NewBranding(pool *pgxpool.Pool, store *settings.Store) *Branding {
	return &Branding{pool: pool, settings: store}
}

// Current returns the configured brand, or the built-in one.
func (b *Branding) Current(ctx context.Context) (BrandingConfig, error) {
	found, err := b.settings.List(ctx, settings.KindBranding)
	if err != nil {
		return BrandingConfig{}, err
	}
	for _, set := range found {
		if set.Name != brandingName {
			continue
		}
		var stored BrandingConfig
		if err := json.Unmarshal(set.Value, &stored); err != nil {
			return BrandingConfig{}, fmt.Errorf("admin: decode branding: %w", err)
		}
		return normalizeBranding(stored)
	}
	return DefaultBranding, nil
}

// Set records what this installation calls and paints itself.
//
// The stored brand is read inside the write so the images it used can be
// dropped once nothing refers to them; read before, two saves racing would
// each delete what the other just pointed at.
func (b *Branding) Set(
	ctx context.Context, by domain.UserID, scope domain.Scope, config BrandingConfig,
) error {
	normalized, err := normalizeBranding(config)
	if err != nil {
		return err
	}
	value, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("admin: encode branding: %w", err)
	}

	set := settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindBranding,
		Name:      brandingName,
		Value:     value,
		Enabled:   true,
		UpdatedBy: string(by),
	}
	var previous BrandingConfig
	return writeFolded(ctx, b.pool, b.settings, folded{
		by: by, scope: scope, action: "branding.changed", target: brandingName, set: set,
		fold: func(stored settings.Setting) (settings.Setting, any, error) {
			previous = BrandingConfig{}
			if len(stored.Value) > 0 {
				if err := json.Unmarshal(stored.Value, &previous); err != nil {
					return settings.Setting{}, nil, fmt.Errorf("admin: decode branding: %w", err)
				}
			}
			return set, brandingDetail(normalized), nil
		},
		then: func(ctx context.Context, conn settings.DB) error {
			if err := requireAssets(ctx, conn, normalized); err != nil {
				return err
			}
			return dropReplaced(ctx, conn, b.settings, previous)
		},
	})
}

func normalizeBranding(config BrandingConfig) (BrandingConfig, error) {
	out := BrandingConfig{
		DisplayName:  strings.TrimSpace(config.DisplayName),
		LogoURL:      strings.TrimSpace(config.LogoURL),
		IconURL:      strings.TrimSpace(config.IconURL),
		PrimaryColor: strings.TrimSpace(config.PrimaryColor),
	}
	if out.DisplayName == "" {
		return BrandingConfig{}, fmt.Errorf("%w: display name is required", ErrBrandingInvalid)
	}
	if len(out.DisplayName) > 80 {
		return BrandingConfig{}, fmt.Errorf("%w: display name is too long", ErrBrandingInvalid)
	}
	if err := checkImageURL(out.LogoURL, "logo"); err != nil {
		return BrandingConfig{}, err
	}
	if err := checkImageURL(out.IconURL, "icon"); err != nil {
		return BrandingConfig{}, err
	}
	if out.PrimaryColor != "" && !hexColour.MatchString(out.PrimaryColor) {
		return BrandingConfig{}, fmt.Errorf("%w: primary colour must be #RRGGBB", ErrBrandingInvalid)
	}
	return out, nil
}

func checkImageURL(raw, field string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > 4096 {
		return fmt.Errorf("%w: %s URL is too long", ErrBrandingInvalid, field)
	}
	if dataImage.MatchString(raw) || brandAssetHash(raw) != "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%w: %s URL is not a URL", ErrBrandingInvalid, field)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("%w: %s URL must be http, https, a data image or an uploaded image", ErrBrandingInvalid, field)
	}
	return nil
}
