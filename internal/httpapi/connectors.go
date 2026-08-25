package httpapi

import (
	"context"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// ListConnectorCatalog exposes the governed connector shapes this release can
// describe. It does not configure an instance and creates no executable tool.
func (s *Server) ListConnectorCatalog(ctx context.Context, _ openapi.ListConnectorCatalogRequestObject) (openapi.ListConnectorCatalogResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermToolClassify); resp != nil {
		return openapi.ListConnectorCatalog403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}

	catalog := connectors.Catalog()
	items := make([]openapi.GovernedConnector, 0, len(catalog))
	for _, c := range catalog {
		item := openapi.GovernedConnector{
			Id:         c.ID,
			Name:       c.Name,
			Category:   openapi.GovernedConnectorCategory(c.Category),
			Summary:    c.Summary,
			Maturity:   openapi.GovernedConnectorMaturity(c.Maturity),
			Guarantees: slices.Clone(c.Guarantees),
			Caveats:    slices.Clone(c.Caveats),
			Operations: connectorOperations(c.Operations),
		}
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b openapi.GovernedConnector) int {
		return strings.Compare(a.Name, b.Name)
	})
	return openapi.ListConnectorCatalog200JSONResponse{Items: items}, nil
}

func connectorOperations(in []connectors.Operation) []openapi.GovernedConnectorOperation {
	out := make([]openapi.GovernedConnectorOperation, 0, len(in))
	for _, op := range in {
		out = append(out, openapi.GovernedConnectorOperation{
			Id:             op.ID,
			Name:           op.Name,
			Summary:        op.Summary,
			Effects:        connectorEffects(op.Effects),
			Approval:       openapi.ConnectorApproval(op.Approval),
			SecretHandling: openapi.ConnectorSecretHandling(op.SecretHandling),
		})
	}
	return out
}

func connectorEffects(in []connectors.Effect) []openapi.ConnectorEffect {
	out := make([]openapi.ConnectorEffect, 0, len(in))
	for _, e := range in {
		out = append(out, openapi.ConnectorEffect(e))
	}
	return out
}
