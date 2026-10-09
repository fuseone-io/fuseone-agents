package connectortools

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/settings"
)

func validElasticsearchInstance() Instance {
	return Instance{
		Connector: "elasticsearch",
		Name:      "gw-logs",
		Enabled:   true,
		Elasticsearch: ElasticsearchConfig{
			BaseURL:  "http://search.internal:9200",
			Username: "reader",
			Index:    "logs-*",
		},
	}
}

func TestValidateElasticsearchConfig_acceptsTheMinimalInstanceAndFillsDefaults(t *testing.T) {
	t.Parallel()
	instance := validElasticsearchInstance()
	if err := ValidateInstanceConfig(instance); err != nil {
		t.Fatalf("ValidateInstanceConfig: %v", err)
	}
	cfg := instance.Elasticsearch
	if elasticsearchField(cfg.IPField, defaultESIPField) != "remote-address" ||
		elasticsearchWindowCap(cfg) != defaultESMaxWindowDays*24 {
		t.Fatalf("defaults did not resolve: %+v", cfg)
	}
}

func TestValidateElasticsearchConfig_refusesWhatMustBeRefused(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*Instance){
		"no url":          func(i *Instance) { i.Elasticsearch.BaseURL = "" },
		"metadata url":    func(i *Instance) { i.Elasticsearch.BaseURL = "http://169.254.169.254/" },
		"no username":     func(i *Instance) { i.Elasticsearch.Username = "" },
		"no index":        func(i *Instance) { i.Elasticsearch.Index = "" },
		"leading star":    func(i *Instance) { i.Elasticsearch.Index = "*" },
		"index spaces":    func(i *Instance) { i.Elasticsearch.Index = "logs *" },
		"bad field":       func(i *Instance) { i.Elasticsearch.IPField = "a b" },
		"window past cap": func(i *Instance) { i.Elasticsearch.MaxWindowDays = 31 },
		"negative window": func(i *Instance) { i.Elasticsearch.MaxWindowDays = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			instance := validElasticsearchInstance()
			mutate(&instance)
			if err := ValidateInstanceConfig(instance); err == nil {
				t.Fatal("ValidateInstanceConfig accepted it")
			}
		})
	}
}

// Basic auth: the username is configuration, the password is the sealed
// token — so the connector requires one, like vault and cloudflare.
func TestValidateInstance_elasticsearchNeedsItsOwnToken(t *testing.T) {
	t.Parallel()
	if !RequiresToken("elasticsearch") {
		t.Fatal("RequiresToken(elasticsearch) = false")
	}
	instance := validElasticsearchInstance()
	if err := ValidateInstance(instance); err == nil {
		t.Fatal("an enabled instance with no password was accepted")
	}
	instance.Token = "s3cr3t"
	if err := ValidateInstance(instance); err != nil {
		t.Fatalf("ValidateInstance with password: %v", err)
	}
}

func TestSettingValue_elasticsearchRoundTrip(t *testing.T) {
	t.Parallel()
	instance := validElasticsearchInstance()
	instance.Elasticsearch.PathField = "url.path"
	instance.Elasticsearch.MaxWindowDays = 14
	value, err := SettingValue(instance)
	if err != nil {
		t.Fatalf("SettingValue: %v", err)
	}
	if strings.Contains(string(value), "CANARY") {
		t.Fatal("the stored value grew a password field")
	}
	back, err := SettingInstance(settings.Setting{
		Name: instance.Name, Value: value, Enabled: true,
	})
	if err != nil {
		t.Fatalf("SettingInstance: %v", err)
	}
	if back.Elasticsearch.Index != "logs-*" || back.Elasticsearch.PathField != "url.path" ||
		back.Elasticsearch.MaxWindowDays != 14 {
		t.Fatalf("round trip lost configuration: %+v", back.Elasticsearch)
	}
	other, err := SettingValue(validCloudflareInstance())
	if err != nil {
		t.Fatalf("SettingValue(cloudflare): %v", err)
	}
	if strings.Contains(string(other), "elasticsearch") {
		t.Fatalf("a cloudflare instance stored an elasticsearch object: %s", other)
	}
}

// The catalog offers exactly the two named queries, both reads.
func TestToolEntries_runtimeElasticsearchOffersOnlyTheNamedQueries(t *testing.T) {
	t.Parallel()
	instance := validElasticsearchInstance()
	instance.HasToken = true
	entries := toolEntriesFor([]Instance{instance})
	if len(entries) != 2 {
		t.Fatalf("entries = %d (%+v), want top_ips and resource_history", len(entries), entries)
	}
	for _, entry := range entries {
		if entry.Effect.String() != "read" {
			t.Fatalf("entry %s effect = %v, want read", entry.ID, entry.Effect)
		}
	}
}
