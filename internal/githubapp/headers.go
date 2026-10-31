package githubapp

import (
	"errors"
	"maps"
	"net/http"
)

// ErrCredentialHeader is a header beside an App that would carry a second
// credential. It names no value; the caller knows which header it sent.
var ErrCredentialHeader = errors.New(
	"github app: headers beside a GitHub App configure the server and cannot carry a credential " +
		"— Authorization and Proxy-Authorization are the App's to set")

// credentialHeaders are the names a header map beside an App may not use,
// canonicalised the way net/http compares them.
var credentialHeaders = []string{"Authorization", "Proxy-Authorization"}

// CheckConfigHeaders refuses a header map that names a credential header, in
// any letter case.
func CheckConfigHeaders(headers map[string]string) error {
	for name := range headers {
		if isCredentialHeader(name) {
			return ErrCredentialHeader
		}
	}
	return nil
}

// ConfigHeaders is headers without any credential header: the runtime's own
// filter, so a row stored before the check, or by a path that skipped it,
// still cannot re-identify an App's request.
func ConfigHeaders(headers map[string]string) map[string]string {
	out := maps.Clone(headers)
	maps.DeleteFunc(out, func(name, _ string) bool { return isCredentialHeader(name) })
	return out
}

func isCredentialHeader(name string) bool {
	canonical := http.CanonicalHeaderKey(name)
	for _, banned := range credentialHeaders {
		if canonical == banned {
			return true
		}
	}
	return false
}
