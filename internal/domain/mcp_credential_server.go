package domain

// MCPCredentialServer is a remote MCP server a person may hold a personal
// credential for (NT-019): one whose scope their tool:read reaches. It names
// no address, which only whoever configures the server reads.
type MCPCredentialServer struct {
	Name string
	// Scope is where the server is stored; the zero scope is the installation.
	Scope Scope
	// Shared says the server holds a credential of its own, used when the
	// person has none.
	Shared bool
}
