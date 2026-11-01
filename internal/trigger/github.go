package trigger

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
)

// A webhook that comes from GitHub is checked GitHub's way.
//
// GitHub never sends the secret. It signs the raw body with it and sends the
// HMAC in X-Hub-Signature-256, names the delivery in X-GitHub-Delivery, and
// says which event it is in X-GitHub-Event, with the action inside the body.
// So the secret has to be kept where it can be read again — sealed, not hashed
// — and the trigger declares which events start a run.

// GitHubSignaturePrefix is the algorithm GitHub writes before the HMAC. The
// older SHA-1 header is never read.
const GitHubSignaturePrefix = "sha256="

// VerifyGitHubSignature reports whether header is the HMAC-SHA256 of body
// under secret, compared in constant time.
func VerifyGitHubSignature(secret, body []byte, header string) bool {
	if len(secret) == 0 || !strings.HasPrefix(header, GitHubSignaturePrefix) {
		return false
	}
	offered, err := hex.DecodeString(strings.TrimPrefix(header, GitHubSignaturePrefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), offered)
}

// GitHubWants reports whether a delivery is one the trigger declared: its
// event alone takes every action, `event.action` only that one.
func GitHubWants(declared []string, event, action string) bool {
	if event == "" {
		return false
	}
	if slices.Contains(declared, event) {
		return true
	}
	return action != "" && slices.Contains(declared, event+"."+action)
}

// GitHubAction reads the action GitHub puts in the body, or "" when the event
// has none. A body that is not JSON has none either: the door refuses it
// before asking.
func GitHubAction(body []byte) string {
	var envelope struct {
		Action any `json:"action"` // any: a non-string action is read as none, not as an error
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	action, _ := envelope.Action.(string)
	return action
}
