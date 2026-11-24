package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

// HTTP header names are case-insensitive, so two spellings of one name are one
// header with two values — and which one is sent depends on map order.
func TestCheckHeaderNames_twoSpellingsOfOneName_areRefusedWithoutTheirValues(t *testing.T) {
	t.Parallel()

	err := domain.CheckHeaderNames(map[string]string{
		"X-MCP-Readonly": "true-secret", "x-mcp-readonly": "false-secret",
	})

	if !errors.Is(err, domain.ErrHeaderCollision) {
		t.Fatalf("err = %v, want ErrHeaderCollision", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v quotes a value", err)
	}
}

func TestCheckHeaderNames_distinctNames_pass(t *testing.T) {
	t.Parallel()

	err := domain.CheckHeaderNames(map[string]string{"X-MCP-Readonly": "true", "X-MCP-Toolsets": "issues"})

	if err != nil {
		t.Fatalf("err = %v, want distinct names accepted", err)
	}
}

/*
A header written for a server is one HTTP can send and that leaves the
protocol alone. A name with a space or a value with a line break is refused at
the write, where the person typing it sees why, rather than at every call; a
header the transport owns (Host, Content-Type, the MCP session) would break the
connection it is meant to authenticate. The value never appears in the error.
*/
func TestCheckWrittenHeaders_whatHTTPCannotSendOrTheTransportOwns_isRefused(t *testing.T) {
	t.Parallel()
	for name, headers := range map[string]map[string]string{
		"a name with a space":  {"api key": "k-secret"},
		"an empty name":        {"": "k-secret"},
		"a name with a colon":  {"apikey:": "k-secret"},
		"a value with a break": {"apikey": "k-secret\r\nX-Injected: 1"},
		"a value with a NUL":   {"apikey": "k-secret\x00"},
		"the host":             {"host": "k-secret"},
		"the content type":     {"Content-Type": "k-secret"},
		"the MCP session":      {"mcp-session-id": "k-secret"},
		"the MCP protocol":     {"MCP-Protocol-Version": "k-secret"},
		"the MCP method":       {"Mcp-Method": "k-secret"},
		"the MCP tool name":    {"mcp-name": "k-secret"},
		"an MCP parameter":     {"Mcp-Param-Region": "k-secret"},
		"a future MCP header":  {"Mcp-Trace": "k-secret"},
		"the resumption point": {"Last-Event-ID": "k-secret"},
		"two spellings":        {"apikey": "k-secret", "APIKEY": "k-secret"},
	} {
		err := domain.CheckWrittenHeaders(headers)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: err = %v quotes a value", name, err)
		}
	}
}

func TestCheckWrittenHeaders_anAPIKeyHeader_passes(t *testing.T) {
	t.Parallel()
	for _, headers := range []map[string]string{
		{"apikey": "k-123"}, {"X-API-Key": "k 123"}, {"Authorization": "Basic dXNlcjpwYXNz"}, {"x_tenant.id": "acme\tmain"},
	} {
		if err := domain.CheckWrittenHeaders(headers); err != nil {
			t.Errorf("%v: err = %v, want accepted", headers, err)
		}
	}
}
