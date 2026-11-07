package connectortools

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/settings"
)

type ConfiguredInstance struct {
	Instance
	ScopeKind settings.ScopeKind
	HasToken  bool
	UpdatedBy string
	UpdatedAt time.Time
}

func (s *Settings) Configured(ctx context.Context) ([]ConfiguredInstance, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	rows, err := s.store.List(ctx, settings.KindConnectorInstance)
	if err != nil {
		return nil, err
	}
	out := make([]ConfiguredInstance, 0, len(rows))
	for _, row := range rows {
		configured, err := ConfiguredFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, configured)
	}
	return out, nil
}

// ConfiguredFrom is what a read says about one stored instance, its token left out.
func ConfiguredFrom(row settings.Setting) (ConfiguredInstance, error) {
	instance, err := SettingInstance(row)
	if err != nil {
		return ConfiguredInstance{}, err
	}
	return ConfiguredInstance{
		Instance: instance, ScopeKind: row.ScopeKind, HasToken: row.HasSecret,
		UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
	}, nil
}

/*
ConfiguredInstances is the non-secret read: every instance, no token.

The credential resolver uses this rather than Instances, which reveals the
token of every enabled instance to answer any question. Choosing a vault does
not require holding the credentials of all of them.
*/
func (s *Settings) ConfiguredInstances(ctx context.Context) ([]ConfiguredInstance, error) {
	return s.Configured(ctx)
}
