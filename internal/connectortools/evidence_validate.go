package connectortools

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/netguard"
)

func validateEvidenceConfig(instance Instance) error {
	cfg := instance.Evidence
	for _, check := range []func(EvidenceConfig) error{
		validateEvidenceSource, validateEvidencePaging, validateEvidenceWindow, validateEvidenceQueries,
	} {
		if err := check(cfg); err != nil {
			return fmt.Errorf("connector: evidence %s %w", instance.Name, err)
		}
	}
	return nil
}

func validateEvidenceSource(cfg EvidenceConfig) error {
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("address must be https")
	}
	if err := netguard.ValidateHTTPURL(cfg.BaseURL); err != nil {
		return errors.New("address cannot target cloud metadata or link-local networks")
	}
	switch {
	case !evidenceHeaderRE.MatchString(cfg.AuthHeader):
		return errors.New("needs a header name for its token")
	case strings.ContainsAny(cfg.AuthPrefix, "\r\n"):
		return errors.New("token prefix must be one line")
	case strings.TrimSpace(cfg.Rows) == "":
		return errors.New("needs the path to the events in a response")
	case cfg.MaxEvents < 0 || cfg.MaxEvents > maxEvidenceEvents:
		return fmt.Errorf("event ceiling must be between 0 and %d", maxEvidenceEvents)
	}
	return nil
}

func validateEvidencePaging(cfg EvidenceConfig) error {
	paging := cfg.Paging
	if !evidenceParamRE.MatchString(paging.LimitParam) ||
		paging.PageSize < 1 || paging.PageSize > maxEvidencePageSize {
		return fmt.Errorf("needs a limit parameter and a page size up to %d", maxEvidencePageSize)
	}
	switch paging.Kind {
	case EvidencePagingOffset:
		if !evidenceParamRE.MatchString(paging.OffsetParam) {
			return errors.New("offset paging needs an offset parameter")
		}
	case EvidencePagingCursor:
		if !evidenceParamRE.MatchString(paging.CursorParam) || paging.NextCursor == "" {
			return errors.New("cursor paging needs a cursor parameter and the path to the next one")
		}
	default:
		return fmt.Errorf("paging must be %q or %q", EvidencePagingOffset, EvidencePagingCursor)
	}
	return nil
}

func validateEvidenceWindow(cfg EvidenceConfig) error {
	window := cfg.Window
	switch window.Kind {
	case EvidenceWindowPeriod:
		if !evidenceParamRE.MatchString(window.PeriodParam) || len(window.Periods) == 0 ||
			window.Periods[0] < 1 || !slices.IsSorted(window.Periods) {
			return errors.New("a period window needs a parameter and ascending periods")
		}
	case EvidenceWindowRange:
		if !evidenceParamRE.MatchString(window.StartParam) || !evidenceParamRE.MatchString(window.EndParam) ||
			(window.Format != EvidenceFormatISO && window.Format != EvidenceFormatEpoch) {
			return errors.New("a range window needs start and end parameters and a format")
		}
	default:
		return fmt.Errorf("window must be %q or %q", EvidenceWindowPeriod, EvidenceWindowRange)
	}
	return nil
}

func validateEvidenceQueries(cfg EvidenceConfig) error {
	if len(cfg.Queries) == 0 {
		return errors.New("declares no queries")
	}
	seen := map[string]bool{}
	for _, query := range cfg.Queries {
		if seen[query.ID] {
			return fmt.Errorf("declares query %q twice", query.ID)
		}
		seen[query.ID] = true
		if err := validateEvidenceQuery(cfg, query); err != nil {
			return fmt.Errorf("query %q %w", query.ID, err)
		}
	}
	control, found := evidenceQueryByID(cfg, cfg.Control)
	if !found || len(control.Parameters) > 0 || control.Windowless {
		return errors.New("control must name a query with no parameters that honours the window")
	}
	return nil
}

func validateEvidenceQuery(cfg EvidenceConfig, query EvidenceQuery) error {
	switch {
	case !evidenceNameRE.MatchString(query.ID):
		return errors.New("has an invalid id")
	case !evidencePathRE.MatchString(query.Path) || strings.Contains(query.Path, ".."):
		return errors.New("needs an absolute path with no query string")
	}
	if err := validateEvidenceFields(cfg, query); err != nil {
		return err
	}
	return validateEvidenceFilters(cfg, query)
}

func validateEvidenceFields(cfg EvidenceConfig, query EvidenceQuery) error {
	if len(query.Fields) == 0 {
		return errors.New("keeps no fields")
	}
	for _, field := range query.Fields {
		if !evidenceFieldRE.MatchString(field) {
			return fmt.Errorf("has an invalid field %q", field)
		}
	}
	if query.IDField != "" && !slices.Contains(query.Fields, query.IDField) {
		return errors.New("must keep its id field")
	}
	if query.TimeField != "" && !slices.Contains(query.Fields, query.TimeField) {
		return errors.New("must keep its time field")
	}
	// Without an id an offset that stopped advancing looks like more events.
	if cfg.Paging.Kind == EvidencePagingOffset && query.IDField == "" {
		return errors.New("needs an id field under offset paging")
	}
	return nil
}

func validateEvidenceFilters(cfg EvidenceConfig, query EvidenceQuery) error {
	used := map[string]bool{}
	reserved := evidenceReservedParams(cfg)
	for _, filter := range query.Filters {
		if !evidenceParamRE.MatchString(filter.Name) || reserved[filter.Name] {
			return fmt.Errorf("has a filter %q that collides with paging or the window", filter.Name)
		}
		for _, match := range evidencePlaceholderRE.FindAllStringSubmatch(filter.Value, -1) {
			used[match[1]] = true
		}
	}
	for _, parameter := range query.Parameters {
		if !evidenceNameRE.MatchString(parameter.Name) || !used[parameter.Name] {
			return fmt.Errorf("declares a parameter %q no filter uses", parameter.Name)
		}
		if parameter.Pattern == "" {
			return fmt.Errorf("parameter %q needs a pattern", parameter.Name)
		}
		if _, err := regexp.Compile(parameter.Pattern); err != nil {
			return fmt.Errorf("parameter %q has an invalid pattern", parameter.Name)
		}
		delete(used, parameter.Name)
	}
	if len(used) > 0 {
		return errors.New("has a filter with a placeholder no parameter declares")
	}
	return nil
}

func evidenceReservedParams(cfg EvidenceConfig) map[string]bool {
	return map[string]bool{
		cfg.Paging.LimitParam: true, cfg.Paging.OffsetParam: true, cfg.Paging.CursorParam: true,
		cfg.Window.PeriodParam: true, cfg.Window.StartParam: true, cfg.Window.EndParam: true,
	}
}
