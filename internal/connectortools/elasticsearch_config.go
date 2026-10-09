package connectortools

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/fuseone/agents/internal/netguard"
)

const (
	defaultESMaxWindowDays = 7
	maxESWindowDays        = 30

	defaultESTimestampField = "@timestamp"
	defaultESIPField        = "remote-address"
	defaultESPathField      = "uri"
	defaultESStatusField    = "status"
)

/*
ElasticsearchConfig configures one governed window over one index pattern.

Everything that shapes the documents — which index, which field holds the
client address, the path, the status — is configuration here, never a
constant and never an argument: this connector ships generic, and the model
only ever supplies values for the named queries, so the one thing it can
never do is choose what to read from or widen what comes back.
*/
type ElasticsearchConfig struct {
	// BaseURL is the cluster endpoint. Plain http is allowed on purpose:
	// the common deployment is in-cluster, like the vault connector's.
	BaseURL string
	// Username pairs with the sealed token, which is the password.
	Username string
	// Index is the fixed index pattern every query runs against. Arguments
	// never choose an index.
	Index string
	// Field mapping, with defaults matching a common gateway access-log
	// shape. The installation's documents decide; the operator writes it.
	TimestampField string
	IPField        string
	PathField      string
	StatusField    string
	// MaxWindowDays caps how far back a query may look. Zero means the
	// default; the ceiling exists because an unbounded window is an
	// unbounded bill on somebody else's cluster.
	MaxWindowDays int
}

var esFieldRE = regexp.MustCompile(`^[A-Za-z0-9@][A-Za-z0-9@._-]{0,254}$`)
var esIndexRE = regexp.MustCompile(`^[a-z0-9][a-z0-9*._-]{0,254}$`)

func validateElasticsearchConfig(instance Instance) error {
	cfg := instance.Elasticsearch
	if err := netguard.ValidateHTTPURL(cfg.BaseURL); err != nil {
		if errors.Is(err, netguard.ErrBlockedAddress) {
			return fmt.Errorf("connector: elasticsearch %s address cannot target cloud metadata or link-local networks", instance.Name)
		}
		return fmt.Errorf("connector: elasticsearch %s address must be http or https", instance.Name)
	}
	switch {
	case strings.TrimSpace(cfg.Username) == "":
		return fmt.Errorf("connector: elasticsearch %s needs a username; the sealed token is its password", instance.Name)
	case !esIndexRE.MatchString(cfg.Index):
		return fmt.Errorf("connector: elasticsearch %s needs an index pattern that names something (a leading wildcard reads every index)", instance.Name)
	case cfg.MaxWindowDays < 0 || cfg.MaxWindowDays > maxESWindowDays:
		return fmt.Errorf("connector: elasticsearch %s window ceiling must be between 0 and %d days", instance.Name, maxESWindowDays)
	}
	for _, field := range []string{
		cfg.TimestampField, cfg.IPField, cfg.PathField, cfg.StatusField,
	} {
		if field != "" && !esFieldRE.MatchString(field) {
			return fmt.Errorf("connector: elasticsearch %s has an invalid field name %q", instance.Name, field)
		}
	}
	return nil
}

// elasticsearchField resolves one mapped field or its default.
func elasticsearchField(configured, fallback string) string {
	if strings.TrimSpace(configured) == "" {
		return fallback
	}
	return configured
}

// elasticsearchWindowCap is the instance's ceiling, in hours.
func elasticsearchWindowCap(cfg ElasticsearchConfig) int {
	days := cfg.MaxWindowDays
	if days <= 0 {
		days = defaultESMaxWindowDays
	}
	return days * 24
}

func storedElasticsearch(cfg *ElasticsearchConfig) ElasticsearchConfig {
	if cfg == nil {
		return ElasticsearchConfig{}
	}
	return *cfg
}

func elasticsearchToStore(cfg ElasticsearchConfig) *ElasticsearchConfig {
	if cfg == (ElasticsearchConfig{}) {
		return nil
	}
	return &cfg
}
