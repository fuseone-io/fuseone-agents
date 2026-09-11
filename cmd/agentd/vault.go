package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/fuseone/agents/internal/vault"
)

// The master key: the one command that produces one, and the one place a
// process opens it.

// keygen prints a master key.
//
// It goes to stdout and nowhere else: the key is never stored by the platform,
// because a platform that can read its own credentials at rest offers an
// attacker with database access nothing to break.
func keygen() error {
	key, err := vault.GenerateKey()
	if err != nil {
		return err
	}
	fmt.Println(key)
	fmt.Fprintf(os.Stderr,
		"\nSet this as %s wherever agentd runs. Losing it means every stored\n"+
			"credential has to be entered again; leaking it means they are all readable.\n",
		vault.KeyEnv)
	return nil
}

// openVault reads the master key. Configuration with a credential in it is
// unreadable without one, so a worker that needs providers needs this.
func openVault() (*vault.Vault, error) {
	// The key id travels with the ciphertext so a future rotation can tell
	// which key sealed a given row.
	v, err := vault.FromEnv("primary")
	if errors.Is(err, vault.ErrNoKey) {
		// Wrapped, so the sentinel survives. The console serves without a key
		// and the worker refuses to start without one, and both decide by
		// asking errors.Is — a message that only reads like the sentinel
		// makes "no key" indistinguishable from "a key that is wrong", which
		// is how a first install stops booting at all.
		return nil, fmt.Errorf(
			"%w: the administration area seals credentials; set %s (agentd version prints how)",
			vault.ErrNoKey, vault.KeyEnv)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", vault.KeyEnv, err)
	}
	return v, nil
}
