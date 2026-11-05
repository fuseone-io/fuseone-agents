package connectortools

import (
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// evidenceCollectArgs is either a fresh collection or a resume, never both.
type evidenceCollectArgs struct {
	Query  string            `json:"query,omitempty"`
	Params map[string]string `json:"params,omitempty"`
	Window *evidenceArgument `json:"window,omitempty"`
	Resume string            `json:"resume,omitempty"`
}

// evidenceArgument is the window a model asks for: a period or a range.
type evidenceArgument struct {
	PeriodSeconds int    `json:"periodSeconds,omitempty"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
}

// evidenceSpan is the window actually used, always stated in absolute UTC.
type evidenceSpan struct {
	Kind          string `json:"kind"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	PeriodSeconds int    `json:"periodSeconds,omitempty"`
}

func decodeEvidenceCollect(raw []byte) (evidenceCollectArgs, bool) {
	var args evidenceCollectArgs
	if !decodeStrict(raw, &args) {
		return evidenceCollectArgs{}, false
	}
	if args.Resume != "" {
		return args, args.Query == "" && args.Params == nil && args.Window == nil
	}
	return args, args.Query != ""
}

/*
evidenceValueAllowed is the floor under every declared pattern.

Patterns are the operator's and one of them will be `.*`. These characters are
how a value stops being a value in the query languages this shape serves, so
none of them reach a source whatever the pattern says. Refusing needs no
knowledge of a vendor's dialect; escaping would need all of them.
*/
func evidenceValueAllowed(value string) bool {
	if value == "" || len(value) > maxEvidenceValue {
		return false
	}
	return !strings.ContainsFunc(value, func(r rune) bool {
		return r == '\'' || r == '"' || r == '`' || r == '\\' || unicode.IsControl(r)
	})
}

// evidenceFilters renders a query's filters, or refuses the whole call.
func evidenceFilters(query EvidenceQuery, params map[string]string) (url.Values, bool) {
	if len(params) != len(query.Parameters) {
		return nil, false
	}
	pairs := make([]string, 0, 2*len(params))
	for _, parameter := range query.Parameters {
		value, supplied := params[parameter.Name]
		if !supplied || !evidenceValueAllowed(value) {
			return nil, false
		}
		// Whole value, whatever anchors the operator did or did not write.
		pattern, err := regexp.Compile(`^(?:` + parameter.Pattern + `)$`)
		if err != nil || !pattern.MatchString(value) {
			return nil, false
		}
		pairs = append(pairs, "{"+parameter.Name+"}", value)
	}
	// One pass, so a value holding a placeholder stays text.
	replacer := strings.NewReplacer(pairs...)
	values := url.Values{}
	for _, filter := range query.Filters {
		values.Set(filter.Name, replacer.Replace(filter.Value))
	}
	return values, true
}

// resolveEvidenceSpan turns what was asked into what will be sent.
func resolveEvidenceSpan(
	window EvidenceWindow, query EvidenceQuery, asked *evidenceArgument, at time.Time,
) (evidenceSpan, bool) {
	if query.Windowless {
		return evidenceSpan{Kind: EvidenceWindowNone}, asked == nil
	}
	if asked == nil {
		return evidenceSpan{}, false
	}
	if window.Kind == EvidenceWindowPeriod {
		return resolveEvidencePeriod(window, *asked, at)
	}
	return resolveEvidenceRange(*asked)
}

// resolveEvidencePeriod takes the shortest accepted period that covers what was
// asked. The answer states the one used, because it is longer than the request.
func resolveEvidencePeriod(window EvidenceWindow, asked evidenceArgument, at time.Time) (evidenceSpan, bool) {
	if asked.From != "" || asked.To != "" || asked.PeriodSeconds < 1 {
		return evidenceSpan{}, false
	}
	index := slices.IndexFunc(window.Periods, func(period int) bool { return period >= asked.PeriodSeconds })
	if index < 0 {
		return evidenceSpan{}, false
	}
	return evidencePeriodSpan(window.Periods[index], at), true
}

func evidencePeriodSpan(period int, at time.Time) evidenceSpan {
	at = at.UTC()
	return evidenceSpan{
		Kind: EvidenceWindowPeriod, PeriodSeconds: period,
		From: at.Add(-time.Duration(period) * time.Second).Format(time.RFC3339),
		To:   at.Format(time.RFC3339),
	}
}

func resolveEvidenceRange(asked evidenceArgument) (evidenceSpan, bool) {
	if asked.PeriodSeconds != 0 {
		return evidenceSpan{}, false
	}
	from, errFrom := time.Parse(time.RFC3339, asked.From)
	to, errTo := time.Parse(time.RFC3339, asked.To)
	if errFrom != nil || errTo != nil || !from.Before(to) {
		return evidenceSpan{}, false
	}
	return evidenceSpan{
		Kind: EvidenceWindowRange,
		From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339),
	}, true
}

// applyEvidenceSpan writes the window into a request in the source's own form.
func applyEvidenceSpan(values url.Values, window EvidenceWindow, span evidenceSpan) {
	switch span.Kind {
	case EvidenceWindowPeriod:
		values.Set(window.PeriodParam, strconv.Itoa(span.PeriodSeconds))
	case EvidenceWindowRange:
		values.Set(window.StartParam, evidenceInstant(span.From, window.Format))
		values.Set(window.EndParam, evidenceInstant(span.To, window.Format))
	}
}

func evidenceInstant(instant, format string) string {
	if format != EvidenceFormatEpoch {
		return instant
	}
	parsed, err := time.Parse(time.RFC3339, instant)
	if err != nil {
		return instant
	}
	return strconv.FormatInt(parsed.Unix(), 10)
}
