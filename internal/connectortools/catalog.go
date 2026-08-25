package connectortools

import (
	"errors"
	"fmt"
	"slices"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/domain"
)

var (
	ErrUnavailable = errors.New("connector unavailable")
	ErrOutOfScope  = errors.New("connector out of scope")
)

func operationEffect(op connectors.Operation) domain.Effect {
	effect := domain.EffectUnknown
	for _, candidate := range op.Effects {
		parsed, err := domain.ParseEffect(string(candidate))
		if err != nil {
			return domain.EffectUnknown
		}
		effect = maxEffect(effect, parsed)
	}
	return effect
}

func maxEffect(a, b domain.Effect) domain.Effect {
	if riskRank(b) > riskRank(a) {
		return b
	}
	return a
}

func riskRank(effect domain.Effect) int {
	switch effect {
	case domain.EffectRead:
		return 1
	case domain.EffectWrite:
		return 2
	case domain.EffectDestructive:
		return 3
	case domain.EffectFinancial:
		return 4
	default:
		return 0
	}
}

func schemaFor(operationID string) (map[string]any, bool) {
	switch operationID {
	case "vault.write_secret":
		return vaultWriteSchema(), true
	case "vault.read_metadata":
		return vaultReadMetadataSchema(), true
	case "vault.revoke_lease":
		return vaultRevokeLeaseSchema(), true
	default:
		return nil, false
	}
}

func toolEntriesFor(instances []Instance) []domain.ToolEntry {
	var out []domain.ToolEntry
	for _, instance := range instances {
		connector, ok := connectorByID(instance.Connector)
		if !ok {
			continue
		}
		for _, op := range connector.Operations {
			id, err := instance.ToolID(op.ID)
			if err != nil {
				continue
			}
			out = append(out, domain.ToolEntry{
				ID: id, Server: fmt.Sprintf("connector:%s/%s", instance.Connector, instance.Name),
				Description: op.Summary, Effect: operationEffect(op),
				Untrusted: operationReads(op), OnSurface: true,
				Native: true, Scope: instance.Scope,
			})
		}
	}
	slices.SortFunc(out, func(a, b domain.ToolEntry) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out
}

func connectorByID(id string) (connectors.Connector, bool) {
	for _, connector := range connectors.Catalog() {
		if connector.ID == id {
			return connector, true
		}
	}
	return connectors.Connector{}, false
}

func operationReads(op connectors.Operation) bool {
	for _, effect := range op.Effects {
		if effect == connectors.EffectRead {
			return true
		}
	}
	return false
}
