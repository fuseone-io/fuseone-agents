// Command factory-measure reads the software-factory pilot's board measures
// from GitHub: time in each lane, and the signals that a later stage or a
// person caught what a role passed (#201, NT-013 § Measured during the
// pilot). It only reads. See README.md.
//
//	go run ./factory/measure report --since 2026-10-05 [--weeks 4] --repo fuseone-io/fuseone-agents --project 2
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Exit codes.
const (
	exitOK    = 0
	exitFail  = 1 // GitHub could not be read
	exitUsage = 2 // bad arguments or a bot map the report cannot use
)

// deps is everything the command takes from outside: the clock included, so
// an open stay's age is the same in every test run.
type deps struct {
	getenv         func(string) string
	stdout, stderr io.Writer
	now            func() time.Time
	http           *http.Client
}

func main() {
	d := deps{getenv: os.Getenv, stdout: os.Stdout, stderr: os.Stderr, now: time.Now}
	os.Exit(run(context.Background(), os.Args[1:], d))
}

func run(ctx context.Context, args []string, d deps) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(d.stderr, "usage: factory-measure report [flags]")
		return exitUsage
	}
	switch args[0] {
	case "report":
		return runReport(ctx, args[1:], d)
	}
	_, _ = fmt.Fprintf(d.stderr, "factory-measure: unknown command %q: report\n", args[0])
	return exitUsage
}

// common are the flags every command takes.
type common struct {
	repo    string
	project int
	bots    string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.repo, "repo", "", "owner/name of the repository (required)")
	fs.IntVar(&c.project, "project", 0, "number of the owner's organization project (required)")
	fs.StringVar(&c.bots, "bots", "", "a bot map to use instead of the shipped factory/measure/bots.yaml")
}

var errUsage = errors.New("usage")

// reader builds the reader from the flags and the environment.
func (c *common) reader(d deps) (reader, error) {
	owner, name, ok := strings.Cut(c.repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") || c.project <= 0 {
		return reader{}, fmt.Errorf("%w: --repo owner/name and --project N are required", errUsage)
	}
	doc := shippedBots
	if c.bots != "" {
		b, err := os.ReadFile(c.bots)
		if err != nil {
			return reader{}, fmt.Errorf("%w: %w", errBots, err)
		}
		doc = b
	}
	bots, err := parseBots(doc)
	if err != nil {
		return reader{}, err
	}
	cl, err := newClient(d.getenv, d.http)
	if err != nil {
		return reader{}, fmt.Errorf("%w: %w", errUsage, err)
	}
	return reader{c: cl, owner: owner, name: name, project: c.project, bots: bots}, nil
}

func fail(d deps, err error) int {
	_, _ = fmt.Fprintln(d.stderr, "factory-measure:", err)
	if errors.Is(err, errUsage) || errors.Is(err, errBots) || errors.Is(err, errPlaceholder) {
		return exitUsage
	}
	return exitFail
}

func runReport(ctx context.Context, args []string, d deps) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(d.stderr)
	var c common
	c.register(fs)
	sinceFlag := fs.String("since", "", "first day to cover, YYYY-MM-DD; its week is the first (required)")
	weeks := fs.Int("weeks", 0, "weeks to cover; 0 runs through the current week")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	since, err := time.Parse(time.DateOnly, *sinceFlag)
	if err != nil || *weeks < 0 {
		return fail(d, fmt.Errorf("%w: --since YYYY-MM-DD is required and --weeks cannot be negative", errUsage))
	}
	r, err := c.reader(d)
	if err == nil {
		err = r.bots.ready()
	}
	if err != nil {
		return fail(d, err)
	}
	now := d.now()
	sc := Scope{Repo: c.repo, Project: c.project, Window: newWindow(since, *weeks, now), Now: now}
	s, err := r.snapshot(ctx, sc.Window.Start)
	if err != nil {
		return fail(d, err)
	}
	render(d.stdout, measure(s, r.bots, sc))
	return exitOK
}
