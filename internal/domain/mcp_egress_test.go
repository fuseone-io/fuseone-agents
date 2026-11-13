package domain

import (
	"net/url"
	"testing"
)

func TestCanonicalOrigin_namesSchemeHostAndPortExactly(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"https://Jira.Example.com":      "https://jira.example.com:443",
		"https://jira.example.com:443/": "https://jira.example.com:443",
		"http://10.24.0.17:8080":        "http://10.24.0.17:8080",
		"http://tools.example.com":      "http://tools.example.com:80",
		"https://[2001:db8::1]:8443":    "https://[2001:db8::1]:8443",
	} {
		got, err := CanonicalOrigin(raw)
		if err != nil || got != want {
			t.Errorf("CanonicalOrigin(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

// An origin is a scheme, a host and a port, nothing else: a path, a query, a
// user or a wildcard would make the list say more than it can hold.
func TestCanonicalOrigin_refusesWhatIsNotAnOrigin(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"", "jira.example.com", "ftp://jira.example.com", "https://",
		"https://jira.example.com/mcp", "https://jira.example.com?x=1", "https://jira.example.com#f",
		"https://user@jira.example.com", "https://*.example.com", "https://jira.example.com:0",
		"https://jira.example.com:99999",
	} {
		if got, err := CanonicalOrigin(raw); err == nil {
			t.Errorf("CanonicalOrigin(%q) = %q, want refused", raw, got)
		}
	}
}

func TestMCPEgress_allowsExactlyTheListedOrigins(t *testing.T) {
	t.Parallel()
	egress := &MCPEgress{Origins: []string{"https://jira.example.com:443"}}
	for raw, want := range map[string]bool{
		"https://jira.example.com/mcp":         true,
		"https://JIRA.example.com:443/x?y=1":   true,
		"http://jira.example.com/mcp":          false,
		"https://jira.example.com:8443/mcp":    false,
		"https://evil.jira.example.com/mcp":    false,
		"https://jira.example.com.evil.io/mcp": false,
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if got := egress.Allows(u); got != want {
			t.Errorf("Allows(%q) = %v, want %v", raw, got, want)
		}
	}
}

// No policy is not an open door: a server held to a policy that lists nothing
// reaches nothing. Only the installation's servers, which carry none, are
// unrestricted.
func TestMCPEgress_anEmptyPolicyAllowsNothing_andNoneIsTheInstallations(t *testing.T) {
	t.Parallel()
	u, _ := url.Parse("https://jira.example.com/mcp")
	if (&MCPEgress{}).Allows(u) {
		t.Error("an empty policy allowed a request")
	}
	var installation *MCPEgress
	if !installation.Allows(u) {
		t.Error("the installation's server was restricted")
	}
}
