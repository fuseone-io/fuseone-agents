package slack_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel/slack"
)

/*
Proving a request came from Slack.

This is the whole of the inbound surface's security. Everything behind it —
resolving a person, deciding an approval, sealing a step into the chain —
treats what arrives as true, so a verifier that is merely usually right is a
platform where a stranger approves payments.

Three properties, and each has been a real vulnerability in somebody's
integration: the signature has to be checked at all, it has to be compared in
constant time, and an old-but-valid request has to be refused. The third is the
one people forget: without it, anybody who ever saw one valid request can
replay it for ever.
*/

const secret = "8f742231b10e8888abcd99yyyzzz85a5"

func TestVerify_signedNow_isAccepted(t *testing.T) {
	t.Parallel()
	body := "payload=%7B%22type%22%3A%22block_actions%22%7D"
	at := time.Now()

	req := httptest.NewRequest("POST", "/hooks/slack", strings.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprint(at.Unix()))
	req.Header.Set("X-Slack-Signature", sign(secret, at, body))

	if err := slack.Verify(req, []byte(body), secret, at); err != nil {
		t.Fatalf("a request Slack signed was refused: %v", err)
	}
}

func TestVerify_bodyAltered_isRefused(t *testing.T) {
	t.Parallel()
	at := time.Now()
	signed := "payload=%7B%22run%22%3A%22run-1%22%7D"
	arrived := "payload=%7B%22run%22%3A%22run-2%22%7D"

	req := httptest.NewRequest("POST", "/hooks/slack", strings.NewReader(arrived))
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprint(at.Unix()))
	req.Header.Set("X-Slack-Signature", sign(secret, at, signed))

	if err := slack.Verify(req, []byte(arrived), secret, at); err == nil {
		t.Fatal("a body that changed under a valid signature was accepted")
	}
}

func TestVerify_replayedLater_isRefused(t *testing.T) {
	t.Parallel()
	// Perfectly signed, and six minutes old. Without this check anybody who
	// ever captured one valid request could approve with it for ever.
	body := "payload=%7B%7D"
	signedAt := time.Now().Add(-6 * time.Minute)

	req := httptest.NewRequest("POST", "/hooks/slack", strings.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprint(signedAt.Unix()))
	req.Header.Set("X-Slack-Signature", sign(secret, signedAt, body))

	err := slack.Verify(req, []byte(body), secret, time.Now())
	if err == nil {
		t.Fatal("a replayed request was accepted")
	}
	if !strings.Contains(err.Error(), "too old") {
		t.Errorf("err = %v, want it to name the reason", err)
	}
}

func TestVerify_noSecretConfigured_refusesRatherThanSkipping(t *testing.T) {
	t.Parallel()
	// An installation that connected a channel and never pasted the signing
	// secret must not have an open endpoint. Refusing is the only safe reading
	// of "not configured yet".
	body := "payload=%7B%7D"
	req := httptest.NewRequest("POST", "/hooks/slack", strings.NewReader(body))

	if err := slack.Verify(req, []byte(body), "", time.Now()); err == nil {
		t.Fatal("an unconfigured channel accepted an unsigned request")
	}
}

func TestVerify_signatureMissing_isRefused(t *testing.T) {
	t.Parallel()
	body := "payload=%7B%7D"
	req := httptest.NewRequest("POST", "/hooks/slack", strings.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprint(time.Now().Unix()))

	if err := slack.Verify(req, []byte(body), secret, time.Now()); err == nil {
		t.Fatal("a request carrying no signature was accepted")
	}
}

func sign(secret string, at time.Time, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "v0:%d:%s", at.Unix(), body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}
