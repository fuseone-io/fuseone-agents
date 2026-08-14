package domain

import (
	"fmt"
	"strings"
)

/*
Installation is the scope above every company.

Creating a company cannot be a permission held inside one: the administrator of
acme would mint another company and grant themselves in it, which is not a
tightening anybody would notice. So there is one scope that reaches everywhere,
and holding it is what the first administrator got by claiming the
installation.

It is a named sentinel and emphatically not the zero value. `Scope{}` is what a
struct starts as, what a failed decode leaves behind, and what half the calls
in this repository pass when a scope is not the point. If that meant
"everything", every one of them would be a grant of everything and the bug
would be silent and total. The zero value has to stay the least authority there
is.

The character is one no identifier may contain, so no company can ever be
registered under this name — which would otherwise be a way to hold the
installation by asking for it.
*/
const Installation CompanyID = "*"

// ValidCompanyID refuses what cannot be a company.
//
// The same shape an area has to have, for the same reason: the id reaches a
// URL path segment, a settings key and a scope written as "company/area", and
// one that has to be escaped to be addressed is one somebody will address
// wrongly. And it refuses the installation sentinel by construction.
func ValidCompanyID(id string) error {
	trimmed := strings.TrimSpace(id)
	switch {
	case trimmed == "":
		return fmt.Errorf("a company needs an identifier")
	case len(trimmed) > 40:
		return fmt.Errorf("company %q: an identifier is at most 40 characters", id)
	}

	for _, r := range trimmed {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !ok {
			return fmt.Errorf(
				"company %q: %q cannot appear in an identifier — lowercase letters, digits and hyphens",
				id, r)
		}
	}
	if strings.HasPrefix(trimmed, "-") || strings.HasSuffix(trimmed, "-") {
		return fmt.Errorf("company %q: an identifier does not start or end with a hyphen", id)
	}
	return nil
}
