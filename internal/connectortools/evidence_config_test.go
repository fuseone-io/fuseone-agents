package connectortools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

func validEvidenceInstance() Instance {
	return Instance{
		Connector: "evidence", Name: "gateway", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "cyber"}, Token: "CANARY-token",
		Evidence: EvidenceConfig{
			BaseURL: "https://source.example.com", AuthHeader: "X-Api-Token",
			Control: "all", Rows: "result", MaxEvents: 1000,
			Refusal: EvidenceMatch{Field: "ok", Value: "0"},
			Gap:     EvidenceMatch{Field: "bypass_traffic", Value: "yes"},
			Paging: EvidencePaging{
				Kind: EvidencePagingOffset, LimitParam: "limit", PageSize: 2, OffsetParam: "offset",
			},
			Window: EvidenceWindow{
				Kind: EvidenceWindowPeriod, PeriodParam: "timeperiod", Periods: []int{3600, 86400},
			},
			Queries: []EvidenceQuery{
				{ID: "all", Description: "Everything, unfiltered.", Path: "/events/all",
					Fields: []string{"_id"}, IDField: "_id"},
				{ID: "by_user", Description: "One user's events.", Path: "/events/page",
					Filters:    []EvidenceFilter{{Name: "query", Value: "user eq '{user}'"}},
					Parameters: []EvidenceParameter{{Name: "user", Pattern: `[a-z.]+@[a-z.]+`}},
					Fields:     []string{"_id", "timestamp", "user", "client_bytes", "bypass_traffic"},
					IDField:    "_id", TimeField: "timestamp"},
				{ID: "sloppy", Description: "A careless pattern.", Path: "/events/page",
					Filters:    []EvidenceFilter{{Name: "query", Value: "user eq '{user}'"}},
					Parameters: []EvidenceParameter{{Name: "user", Pattern: `.*`}},
					Fields:     []string{"_id"}, IDField: "_id"},
			},
		},
	}
}

func TestValidateEvidence_acceptsAWellFormedInstance(t *testing.T) {
	t.Parallel()
	if err := ValidateInstance(validEvidenceInstance()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateEvidence_refusesWhatCouldNotRunHonestly(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(*EvidenceConfig){
		"plain http":         func(c *EvidenceConfig) { c.BaseURL = "http://source.example.com" },
		"metadata address":   func(c *EvidenceConfig) { c.BaseURL = "https://169.254.169.254" },
		"no header":          func(c *EvidenceConfig) { c.AuthHeader = "" },
		"header injection":   func(c *EvidenceConfig) { c.AuthHeader = "X-A\r\nX-B" },
		"prefix injection":   func(c *EvidenceConfig) { c.AuthPrefix = "Bearer\r\nX-B: y " },
		"no rows path":       func(c *EvidenceConfig) { c.Rows = "" },
		"no queries":         func(c *EvidenceConfig) { c.Queries = nil },
		"unknown control":    func(c *EvidenceConfig) { c.Control = "nope" },
		"filtered control":   func(c *EvidenceConfig) { c.Control = "by_user" },
		"windowless control": func(c *EvidenceConfig) { c.Queries[0].Windowless = true },
		"duplicate query":    func(c *EvidenceConfig) { c.Queries[1].ID = "all" },
		"path traversal":     func(c *EvidenceConfig) { c.Queries[1].Path = "/events/../admin" },
		"path with query":    func(c *EvidenceConfig) { c.Queries[1].Path = "/events?x=1" },
		"relative path":      func(c *EvidenceConfig) { c.Queries[1].Path = "events" },
		"bad pattern":        func(c *EvidenceConfig) { c.Queries[1].Parameters[0].Pattern = "(" },
		"empty pattern":      func(c *EvidenceConfig) { c.Queries[1].Parameters[0].Pattern = "" },
		"unused parameter":   func(c *EvidenceConfig) { c.Queries[1].Filters[0].Value = "user eq 'x'" },
		"undeclared placeholder": func(c *EvidenceConfig) {
			c.Queries[1].Filters[0].Value = "user eq '{user}' and host eq '{host}'"
		},
		"filter shadows paging": func(c *EvidenceConfig) { c.Queries[1].Filters[0].Name = "offset" },
		"filter shadows window": func(c *EvidenceConfig) { c.Queries[1].Filters[0].Name = "timeperiod" },
		"no fields":             func(c *EvidenceConfig) { c.Queries[1].Fields = nil },
		"id not kept":           func(c *EvidenceConfig) { c.Queries[1].IDField = "other" },
		"time not kept":         func(c *EvidenceConfig) { c.Queries[1].TimeField = "other" },
		"offset without id":     func(c *EvidenceConfig) { c.Queries[1].IDField = "" },
		"unknown paging":        func(c *EvidenceConfig) { c.Paging.Kind = "page" },
		"offset no param":       func(c *EvidenceConfig) { c.Paging.OffsetParam = "" },
		"cursor no path": func(c *EvidenceConfig) {
			c.Paging = EvidencePaging{Kind: EvidencePagingCursor, LimitParam: "limit", PageSize: 2, CursorParam: "cursor"}
		},
		"no page size":     func(c *EvidenceConfig) { c.Paging.PageSize = 0 },
		"huge ceiling":     func(c *EvidenceConfig) { c.MaxEvents = maxEvidenceEvents + 1 },
		"unknown window":   func(c *EvidenceConfig) { c.Window.Kind = "sliding" },
		"no periods":       func(c *EvidenceConfig) { c.Window.Periods = nil },
		"unordered period": func(c *EvidenceConfig) { c.Window.Periods = []int{86400, 3600} },
		"range no format": func(c *EvidenceConfig) {
			c.Window = EvidenceWindow{Kind: EvidenceWindowRange, StartParam: "from", EndParam: "to"}
		},
	} {
		instance := validEvidenceInstance()
		edit(&instance.Evidence)
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestValidateEvidence_needsAToken(t *testing.T) {
	t.Parallel()
	instance := validEvidenceInstance()
	instance.Token = ""
	if err := ValidateInstance(instance); err == nil {
		t.Fatal("an evidence instance with no token was accepted")
	}
}

// Config fields die in the middle layer: the whole declaration has to survive
// the trip through the stored value, and nothing here injects Config directly.
func TestEvidenceConfig_survivesTheSettingRow(t *testing.T) {
	t.Parallel()
	want := validEvidenceInstance()
	value, err := SettingValue(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := SettingInstance(settings.Setting{
		Name: want.Name, Enabled: true, Value: value,
		ScopeKind: settings.ScopeArea, Scope: domain.Scope{Company: "acme", Area: "cyber"},
	})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(want.Evidence)
	b, _ := json.Marshal(got.Evidence)
	if string(a) != string(b) {
		t.Fatalf("stored\n %s\nread\n %s", a, b)
	}
}

func TestSettingValue_otherConnectorsCarryNoEvidenceObject(t *testing.T) {
	t.Parallel()
	value, err := SettingValue(validElasticsearchInstance())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(value), "evidence") {
		t.Fatalf("value = %s", value)
	}
}

func TestCloneInstance_sharesNoEvidenceDeclaration(t *testing.T) {
	t.Parallel()
	original := validEvidenceInstance()
	clone := cloneInstance(original)
	clone.Evidence.Queries[1].Parameters[0].Pattern = "changed"
	clone.Evidence.Queries[1].Filters[0].Value = "changed"
	clone.Evidence.Queries[1].Fields[0] = "changed"
	clone.Evidence.Window.Periods[0] = 1
	query := original.Evidence.Queries[1]
	if query.Parameters[0].Pattern == "changed" || query.Filters[0].Value == "changed" ||
		query.Fields[0] == "changed" || original.Evidence.Window.Periods[0] == 1 {
		t.Fatalf("clone aliases the original: %+v", original.Evidence)
	}
}
