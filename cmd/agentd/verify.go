package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/fuseone/agents/internal/export"
)

/*
verifyCmd checks a signed export.

It deliberately needs nothing: no database, no credential, no network. An
auditor who was handed a file and this binary can check it on a laptop that
has never seen the installation, and that is the entire point of AU-12 — an
export somebody has to ask us about is an export they are trusting us for.

What it cannot tell them is whether the key that signed it is ours. Nothing
can, from inside the file: the fingerprint is printed so they can compare it
against what the installation publishes, and that comparison is theirs to make.
*/
func verifyCmd(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: agentd verify <file>   (or - for standard input)")
	}

	raw, err := read(args[0])
	if err != nil {
		return err
	}
	bundle, err := export.Decode(raw)
	if err != nil {
		return err
	}

	if err := export.Verify(bundle); err != nil {
		// Printed rather than returned as a fatal log line: somebody is
		// reading this in a terminal to find out an answer, and the answer is
		// the message.
		fmt.Printf("FAILED — %v\n", err)
		os.Exit(1)
	}

	first, last := bundle.Steps[0], bundle.Steps[len(bundle.Steps)-1]
	fmt.Printf("OK — %d steps, chain intact, signature valid\n", len(bundle.Steps))
	fmt.Printf("  company     %s\n", bundle.Company)
	fmt.Printf("  exported    %s\n", bundle.ExportedAt.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  first step  %s seq %d at %s\n", first.RunID, first.Seq, first.At.Format("2006-01-02 15:04:05"))
	fmt.Printf("  last step   %s seq %d at %s\n", last.RunID, last.Seq, last.At.Format("2006-01-02 15:04:05"))
	fmt.Printf("  signed by   %s\n", bundle.Fingerprint())
	fmt.Println()
	fmt.Println("Compare the fingerprint against the key the installation publishes.")
	fmt.Println("Nothing in the file can tell you it is theirs.")
	return nil
}

func read(path string) ([]byte, error) {
	if path == "-" {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read standard input: %w", err)
		}
		return raw, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return raw, nil
}
