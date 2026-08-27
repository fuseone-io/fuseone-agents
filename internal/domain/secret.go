package domain

import (
	"regexp"
	"strings"
	"unicode"
)

/*
SecretRisk is what a piece of text looks like, not what it is.

Two levels because the mistakes differ. A private key header or a complete token
in a known format is not a judgement call — nothing else looks like that, and
letting one through writes a live credential into a record built to be read
back, quoted to a model, and kept for years. Everything else is a guess: long
random-looking text is a password, a hash, a correlation id or somebody's
example, and a platform that refused all of them would teach people to work
around it.

So one is refused and the other is answered with a question.
*/
type SecretRisk string

const (
	SecretNone      SecretRisk = ""
	SecretSuspected SecretRisk = "suspected"
	SecretCertain   SecretRisk = "certain"
)

/*
LooksLikeSecret is the worst of what these fields look like.

It never returns what it found. The caller reports a risk and a field name and
nothing else: an error that quoted the token would copy it into a log, an audit
event and a bug report, which is three more places than the one it was already
in.
*/
func LooksLikeSecret(values ...string) SecretRisk {
	worst := SecretNone
	for _, value := range values {
		switch riskOf(value) {
		case SecretCertain:
			return SecretCertain
		case SecretSuspected:
			worst = SecretSuspected
		}
	}
	return worst
}

// certainSecret are the shapes nothing else has. A private key header names
// itself, and each token format below is issued by one service in one layout.
var certainSecret = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}`),
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\b[sr]k_live_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),
	regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`),
}

/*
suspectSecret are shapes that are sometimes a secret and often not.

The armour header without PRIVATE KEY is the one worth naming: a public
certificate opens exactly that way, and refusing it would refuse somebody
recording which certificate an incident was about. The prefixes are the same
formats as above, cut short — a token somebody truncated before pasting is still
a token they nearly pasted.
*/
var suspectSecret = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]+-----`),
	regexp.MustCompile(`\b(gh[pousr]_|xox[baprs]-|sk_live_|rk_live_|sk-ant-|AKIA)`),
}

func riskOf(value string) SecretRisk {
	for _, shape := range certainSecret {
		if shape.MatchString(value) {
			return SecretCertain
		}
	}
	for _, shape := range suspectSecret {
		if shape.MatchString(value) {
			return SecretSuspected
		}
	}
	for _, word := range strings.Fields(value) {
		if opaqueToken(word) {
			return SecretSuspected
		}
	}
	return SecretNone
}

// minOpaqueToken is where "long random string" starts. Below it the false
// positives are ordinary words and identifiers; above it, almost nothing a
// person types by hand reaches this length without a keyboard mashing it.
const minOpaqueToken = 32

/*
opaqueToken is a long run of characters with no shape a reader could use.

Deliberately blind to hexadecimal and to UUIDs. A digest is sixty-four hex
characters of pure entropy and is exactly what a memory is supposed to carry —
it is how a citation names the bytes it cites — and a rule that could not tell
one from a password would refuse the platform's own evidence. Same for a run id.

What is left is mixed-case-and-digits text long enough that nobody chose it,
which is what generated credentials look like when they carry no prefix to
recognise them by.
*/
func opaqueToken(word string) bool {
	word = strings.Trim(word, ".,;:!?\"'()[]{}")
	if len(word) < minOpaqueToken || isHex(word) || isUUID(word) {
		return false
	}
	var lower, upper, digit int
	for _, r := range word {
		switch {
		case unicode.IsLower(r):
			lower++
		case unicode.IsUpper(r):
			upper++
		case unicode.IsDigit(r):
			digit++
		case !strings.ContainsRune("+/=_-", r):
			// A character no token alphabet uses. Prose, a path, a sentence
			// somebody wrote — not one opaque run.
			return false
		}
	}
	return lower > 0 && upper > 0 && digit > 0
}

func isHex(word string) bool {
	for _, r := range word {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

var uuidShape = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(word string) bool { return uuidShape.MatchString(word) }
