package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

/*
Proving a request came from Slack.

This is the whole of the inbound surface's security. Everything behind it —
resolving a person, deciding an approval, sealing a step into the hash chain —
treats what arrives as true.

The scheme is Slack's: HMAC-SHA256 over "v0:{timestamp}:{body}" with the
signing secret, compared against the X-Slack-Signature header. Three things
have to be right and each has been somebody's vulnerability: the signature is
checked, it is compared in constant time, and a request older than the window
is refused however well it is signed — otherwise anybody who ever captured one
valid request can replay it for ever.

The signing secret is not the bot token. It belongs in the vault rather than in
the hashed-secret column a webhook trigger uses, because HMAC needs the secret
itself and a hash cannot produce one.
*/

// Window is how old a signed request may be.
//
// Slack's own recommendation. Wide enough for a slow network and a clock a
// little out, narrow enough that a captured request stops working before
// anybody can use it.
const Window = 5 * time.Minute

// ErrUnsigned means the request carried no signature, or the installation has
// no secret to check one against. Both are refusals rather than skips: an
// endpoint that let a request through because it was not configured yet would
// be an open door with an approval behind it.
var ErrUnsigned = errors.New("slack: unsigned request")

// Verify answers whether this request is Slack's.
//
// The body is passed in rather than read here: it has already been consumed to
// be verified, and a verifier that re-read it would either fail or, worse,
// verify one set of bytes while the handler acted on another.
func Verify(r *http.Request, body []byte, secret string, now time.Time) error {
	if secret == "" {
		return fmt.Errorf("%w: this channel has no signing secret", ErrUnsigned)
	}

	signature := r.Header.Get("X-Slack-Signature")
	if signature == "" {
		return fmt.Errorf("%w: no signature header", ErrUnsigned)
	}

	stamp := r.Header.Get("X-Slack-Request-Timestamp")
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: unreadable timestamp", ErrUnsigned)
	}
	if age := now.Sub(time.Unix(seconds, 0)); age > Window || age < -Window {
		// Named, because it is the check somebody removes when a request
		// mysteriously fails and their machine's clock is wrong.
		return fmt.Errorf("%w: signed %s ago, which is too old", ErrUnsigned, age.Round(time.Second))
	}

	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "v0:%s:%s", stamp, body)
	want := "v0=" + hex.EncodeToString(mac.Sum(nil))

	// Constant time: a comparison that returned early would leak the signature
	// one byte at a time to anybody willing to send enough requests.
	if !hmac.Equal([]byte(want), []byte(signature)) {
		return fmt.Errorf("%w: the signature does not match", ErrUnsigned)
	}
	return nil
}
