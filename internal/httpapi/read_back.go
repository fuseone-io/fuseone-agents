package httpapi

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// mcpServerConfig is a server as it was configured, before anything observed
// about it is added.
func mcpServerConfig(srv domain.MCPServer) openapi.MCPServer {
	server := openapi.MCPServer{
		Name: srv.Name, Enabled: srv.Enabled,
		Transport:          ptr(openapi.Transport(srv.TransportOf())),
		ProtocolMode:       ptr(openapi.MCPProtocolMode(srv.MCPProtocolModeOf())),
		RateLimit:          rateLimitToResponse(srv.RateLimit),
		CallTimeoutSeconds: srv.CallTimeout,
		Cache:              resultCacheToResponse(srv.Cache),
		HasSecret:          ptr(srv.HasSecret),
		HasOAuth:           ptr(srv.HasOAuth),
		GithubApp:          githubAppToResponse(srv.GitHubApp),
		HasVariables:       ptr(srv.HasVariables),
		HasConfigFile:      ptr(srv.HasConfigFile),
		Managed:            ptr(true),
		UpdatedBy:          ptr(srv.UpdatedBy), UpdatedAt: ptr(srv.UpdatedAt),
		// What was brought in, and who accepted running it. Both are
		// stored and neither reached the screen, so a server somebody had
		// narrowed read back as all-in — and saving anything from that
		// screen would have widened it back.
		//
		// The pointer is the whole point: nil is "nobody has chosen", and
		// an empty list is "chosen, and none of it".
		Surface:               srv.Surface,
		AcceptsLocalExecution: ptr(srv.AcceptsLocalExecution),
		StdioEgress:           stdioEgressToResponse(srv.StdioEgress),
	}
	// Absent rather than null: the schema says array, and a pointer to a nil
	// slice encodes as neither.
	if len(srv.Args) > 0 {
		server.Args = &srv.Args
	}
	server.Command = someString(srv.Command)
	server.Url = someString(srv.URL)
	if srv.ConfigFileEnv != nil {
		server.ConfigFileEnv = someString(*srv.ConfigFileEnv)
	}
	return server
}

func modelProviderConfig(p domain.ModelProvider) openapi.ModelProvider {
	return openapi.ModelProvider{
		Name: p.Name, Kind: openapi.ModelProviderKind(p.Kind), BaseUrl: p.BaseURL,
		// An empty array and never null: a provider with no list is a
		// provider that offers none, and the schema says array or absent.
		Models:               ptr(append([]string{}, p.Models...)),
		AutoToolChoiceModels: ptr(append([]string{}, p.AutoToolChoiceModels...)),
		// Only whether a credential exists. The credential itself never
		// leaves the vault through this API.
		Enabled: p.Enabled, HasKey: p.HasKey,
		UpdatedBy: ptr(p.UpdatedBy), UpdatedAt: ptr(p.UpdatedAt),
	}
}
