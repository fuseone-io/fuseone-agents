package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// shippedBots is the versioned map, factory/measure/bots.yaml.
//
//go:embed bots.yaml
var shippedBots []byte

var (
	errBots        = errors.New("bot map")
	errPlaceholder = errors.New("bot map has placeholders")
)

const placeholderMark = "PLACEHOLDER"

// Bots maps each role to the login of its GitHub App's bot. It is the only
// way an actor becomes a role: a Bot it does not list is unmapped, never
// matched by a near name.
type Bots struct {
	byRole  map[Owner]string
	byLogin map[string]Owner
}

type botsDoc struct {
	Bots map[string]string `yaml:"bots"`
}

// parseBots reads and checks a map: every role listed exactly once, each with
// its own `<slug>[bot]` login. Placeholders parse, so the canary can run
// before the map is complete; ready says whether a report may.
func parseBots(doc []byte) (Bots, error) {
	var d botsDoc
	dec := yaml.NewDecoder(bytes.NewReader(doc))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		return Bots{}, fmt.Errorf("%w: %w", errBots, err)
	}
	b := Bots{byRole: map[Owner]string{}, byLogin: map[string]Owner{}}
	for role, login := range d.Bots {
		if err := b.add(Owner(role), strings.TrimSpace(login)); err != nil {
			return Bots{}, err
		}
	}
	for _, r := range roles() {
		if _, ok := b.byRole[r]; !ok {
			return Bots{}, fmt.Errorf("%w: role %s is not listed", errBots, r)
		}
	}
	return b, nil
}

func (b Bots) add(role Owner, login string) error {
	switch {
	case !role.isRole():
		return fmt.Errorf("%w: %q is not a factory role", errBots, role)
	case !strings.HasSuffix(login, "[bot]") || len(login) == len("[bot]"):
		return fmt.Errorf("%w: %s: %q is not an App bot login (<app-slug>[bot])", errBots, role, login)
	}
	if other, taken := b.byLogin[login]; taken {
		return fmt.Errorf("%w: %s and %s share the login %s", errBots, other, role, login)
	}
	b.byRole[role] = login
	b.byLogin[login] = role
	return nil
}

// placeholders are the roles whose login the operator has not filled in yet.
func (b Bots) placeholders() []string {
	var out []string
	for _, r := range roles() {
		if strings.Contains(b.byRole[r], placeholderMark) {
			out = append(out, string(r))
		}
	}
	return out
}

// ready refuses a map that still holds placeholders: a role whose bot is a
// placeholder would have all its work reported as an unmapped actor, and the
// table would read as that role having done nothing.
func (b Bots) ready() error {
	if p := b.placeholders(); len(p) > 0 {
		return fmt.Errorf("%w for %s: fill in factory/measure/bots.yaml", errPlaceholder, strings.Join(p, ", "))
	}
	return nil
}

// login is the role's bot.
func (b Bots) login(r Owner) string { return b.byRole[r] }

// classify says whose act an actor's is. A Bot is a role only by an exact
// login in the map; a User is a person; anything else (an unknown Bot, a
// deleted account, a Mannequin) is unmapped.
func (b Bots) classify(a Actor) Owner {
	role, listed := b.byLogin[a.Login]
	switch {
	case a.Type == "Bot" && listed && !strings.Contains(a.Login, placeholderMark):
		return role
	case a.Type == "User" && !listed:
		return OwnerPerson
	}
	return OwnerUnmapped
}
