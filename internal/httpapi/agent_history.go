package httpapi

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
readableHistory is every version a reader may read, newest first.

Only those: an agent republished from another area keeps its old versions there, and listing
them would tell a reader of the new area what exists where they have no claim. A version is
read where it was published, so it is listed by the same rule.
*/
func readableHistory(versions []domain.AgentSummary, visible []domain.Scope) []openapi.AgentVersion {
	out := make([]openapi.AgentVersion, 0, len(versions))
	for _, v := range versions {
		if !readable(v.Scope, visible) {
			continue
		}
		version := openapi.AgentVersion{
			VersionId: string(v.VersionID), PublishedAt: v.PublishedAt, Latest: v.Latest,
		}
		if v.PublishedBy != "" {
			version.PublishedBy = ptr(string(v.PublishedBy))
		}
		out = append(out, version)
	}
	return out
}
