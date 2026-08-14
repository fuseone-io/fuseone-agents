package auth_test

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/auth"
)

/*
The administrator's own password.

Break-glass, not the way people sign in: identity comes from the customer's
provider and always will. This is the one account that works when the provider
does not — or does not exist yet, which is the state every installation starts
in and the one an operator can be locked out by.
*/

func TestHashPassword_theSamePasswordTwice_givesDifferentHashes(t *testing.T) {
	t.Parallel()

	first, err := auth.HashPassword("uma senha bem comprida")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	second, err := auth.HashPassword("uma senha bem comprida")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Salted. Two administrators who chose the same password must not be
	// visible as having done so to anybody who reads the table.
	if first == second {
		t.Fatal("the same password produced the same hash twice")
	}
	if !auth.PasswordMatches(first, "uma senha bem comprida") {
		t.Error("the password does not verify against its own hash")
	}
}

func TestPasswordMatches_theWrongPassword_isRefused(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("uma senha bem comprida")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if auth.PasswordMatches(hash, "uma senha bem comprido") {
		t.Error("a password one letter off was accepted")
	}
}

// The parameters travel with the hash, so raising the cost later does not
// invalidate every password already set: an old hash still says how to check
// it, and the next sign-in can write a stronger one.
func TestHashPassword_carriesItsOwnParameters(t *testing.T) {
	t.Parallel()

	hash, err := auth.HashPassword("uma senha bem comprida")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$") {
		t.Errorf("hash = %q, want it to name its own algorithm", hash)
	}
	if strings.Count(hash, "$") != 3 {
		t.Errorf("hash = %q, want algorithm, cost, salt and key", hash)
	}
}

// A stored value nobody can parse is not a reason to let somebody in.
func TestPasswordMatches_aHashThatIsNotOne_isRefused(t *testing.T) {
	t.Parallel()

	for _, broken := range []string{"", "pbkdf2-sha256$", "$$$", "plaintext",
		"pbkdf2-sha256$notanumber$c2FsdA$a2V5"} {
		if auth.PasswordMatches(broken, "uma senha bem comprida") {
			t.Errorf("%q was accepted as a hash", broken)
		}
	}
}

/*
A short password on a console reachable from a browser is the whole attack.

The floor is length rather than a character-class rule: "P@ssw0rd!" satisfies
every such rule and is on every list, and the rules mostly teach people to put
a 1 at the end.
*/
func TestHashPassword_tooShort_isRefused(t *testing.T) {
	t.Parallel()

	if _, err := auth.HashPassword("curta12"); err == nil {
		t.Error("a seven-character password was accepted")
	}
	if _, err := auth.HashPassword(strings.Repeat("a", auth.MinPasswordLength)); err != nil {
		t.Errorf("a password at the floor was refused: %v", err)
	}
}
