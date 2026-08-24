package egressmetrics

import "sort"

const (
	CodeOther                  = "other"
	CodeUnauthorized           = "stdio_egress_unauthorized"
	CodeBadRequest             = "stdio_egress_bad_request"
	CodeDestinationDenied      = "stdio_egress_destination_denied"
	CodeMetadataRefused        = "stdio_egress_metadata_refused"
	CodeDestinationUnavailable = "stdio_egress_destination_unavailable"
)

var codes = map[string]bool{
	CodeUnauthorized:           true,
	CodeBadRequest:             true,
	CodeDestinationDenied:      true,
	CodeMetadataRefused:        true,
	CodeDestinationUnavailable: true,
}

// Code bounds a stdio egress code before it can become a metric label or UI bucket.
func Code(code string) string {
	if codes[code] {
		return code
	}
	return CodeOther
}

// Codes returns the stable stdio egress vocabulary used by metrics and runtime views.
func Codes() []string {
	out := make([]string, 0, len(codes)+1)
	for code := range codes {
		out = append(out, code)
	}
	out = append(out, CodeOther)
	sort.Strings(out)
	return out
}
