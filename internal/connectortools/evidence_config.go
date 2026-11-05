package connectortools

import (
	"regexp"
	"slices"
)

const (
	EvidencePagingOffset = "offset"
	EvidencePagingCursor = "cursor"

	EvidenceWindowPeriod = "period"
	EvidenceWindowRange  = "range"
	// EvidenceWindowNone is reported for a windowless query. It is never
	// declared as an instance's window.
	EvidenceWindowNone = "none"

	EvidenceFormatISO   = "iso"
	EvidenceFormatEpoch = "epoch"

	defaultEvidenceEvents = 200_000
	// maxEvidenceEvents bounds what one collection may hold in the content
	// store. It belongs here rather than to the operator alone, because every
	// event is a row's worth of somebody's activity under the run's retention.
	maxEvidenceEvents   = 1_000_000
	maxEvidencePageSize = 10_000
	maxEvidenceValue    = 256
)

/*
EvidenceConfig declares one source of events.

Everything that names a vendor's API is here, as configuration: the connector
holds no vendor. A model chooses a query and supplies values for its parameters;
it never chooses a path, a field or a page size, so the one thing it cannot do
is widen what is read or what comes back.
*/
type EvidenceConfig struct {
	// BaseURL is the source's API root. https only: the token crosses a network
	// the installation does not own, unlike an in-cluster connector.
	BaseURL string `json:"baseUrl"`
	// AuthHeader and AuthPrefix are how the sealed token travels.
	AuthHeader string `json:"authHeader"`
	AuthPrefix string `json:"authPrefix,omitempty"`
	// Control names the unfiltered query that proves the source answers at all.
	Control string `json:"control"`
	// Rows is the dotted path to the array of events in a response body.
	Rows string `json:"rows"`
	// Refusal is what a query refused inside a 200 response looks like.
	Refusal EvidenceMatch `json:"refusal,omitzero"`
	// Gap marks an event the source saw but could not measure.
	Gap       EvidenceMatch   `json:"gap,omitzero"`
	Paging    EvidencePaging  `json:"paging"`
	Window    EvidenceWindow  `json:"window"`
	MaxEvents int             `json:"maxEvents,omitempty"`
	Queries   []EvidenceQuery `json:"queries"`
}

// EvidenceMatch is one field holding one value. Zero matches nothing.
type EvidenceMatch struct {
	Field string `json:"field,omitempty"`
	Value string `json:"value,omitempty"`
}

type EvidencePaging struct {
	Kind       string `json:"kind"`
	LimitParam string `json:"limitParam"`
	PageSize   int    `json:"pageSize"`
	// OffsetParam is for offset paging.
	OffsetParam string `json:"offsetParam,omitempty"`
	// CursorParam and NextCursor are for cursor paging: where the cursor goes,
	// and the dotted path to the next one in a response body.
	CursorParam string `json:"cursorParam,omitempty"`
	NextCursor  string `json:"nextCursor,omitempty"`
}

type EvidenceWindow struct {
	Kind string `json:"kind"`
	// PeriodParam and Periods are for a source that only accepts fixed relative
	// periods. A value outside the list is how some sources answer with an
	// empty success, so the list is the operator's to get right and the control
	// is what catches it.
	PeriodParam string `json:"periodParam,omitempty"`
	Periods     []int  `json:"periods,omitempty"`
	// StartParam, EndParam and Format are for an absolute range.
	StartParam string `json:"startParam,omitempty"`
	EndParam   string `json:"endParam,omitempty"`
	Format     string `json:"format,omitempty"`
}

// EvidenceQuery is one named read.
type EvidenceQuery struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Path        string `json:"path"`
	// Filters are query-string pairs; a Value may hold {parameter} placeholders.
	Filters    []EvidenceFilter    `json:"filters,omitempty"`
	Parameters []EvidenceParameter `json:"parameters,omitempty"`
	// Fields is the allowlist kept from each event. Everything else is dropped
	// before anything is stored.
	Fields    []string `json:"fields"`
	IDField   string   `json:"idField,omitempty"`
	TimeField string   `json:"timeField,omitempty"`
	// Windowless is a lookup, such as finding a device, where a time filter
	// would mean something else entirely.
	Windowless bool `json:"windowless,omitempty"`
}

type EvidenceFilter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// EvidenceParameter is always required: a query with a filter missing is a
// different query, and it gets its own name.
type EvidenceParameter struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

var (
	evidenceFieldRE       = regexp.MustCompile(`^[A-Za-z0-9_@][A-Za-z0-9@._-]{0,254}$`)
	evidenceNameRE        = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	evidenceHeaderRE      = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	evidenceParamRE       = regexp.MustCompile(`^[A-Za-z0-9_.\[\]-]{1,64}$`)
	evidencePathRE        = regexp.MustCompile(`^(/[A-Za-z0-9_.~-]+)+$`)
	evidencePlaceholderRE = regexp.MustCompile(`\{([^{}]*)\}`)
)

func evidenceQueryByID(cfg EvidenceConfig, id string) (EvidenceQuery, bool) {
	for _, query := range cfg.Queries {
		if query.ID == id {
			return query, true
		}
	}
	return EvidenceQuery{}, false
}

func evidenceCeiling(cfg EvidenceConfig) int64 {
	if cfg.MaxEvents == 0 {
		return defaultEvidenceEvents
	}
	return int64(cfg.MaxEvents)
}

func storedEvidence(cfg *EvidenceConfig) EvidenceConfig {
	if cfg == nil {
		return EvidenceConfig{}
	}
	return *cfg
}

func evidenceToStore(cfg EvidenceConfig) *EvidenceConfig {
	if cfg.BaseURL == "" && len(cfg.Queries) == 0 {
		return nil
	}
	return &cfg
}

func cloneEvidence(in EvidenceConfig) EvidenceConfig {
	out := in
	out.Window.Periods = slices.Clone(in.Window.Periods)
	out.Queries = slices.Clone(in.Queries)
	for i := range out.Queries {
		out.Queries[i].Filters = slices.Clone(in.Queries[i].Filters)
		out.Queries[i].Parameters = slices.Clone(in.Queries[i].Parameters)
		out.Queries[i].Fields = slices.Clone(in.Queries[i].Fields)
	}
	return out
}
