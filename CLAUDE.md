# FuseOne Agents — Core Engineering Guidelines

Governed AI-agent platform, installed in the customer's environment.
Functional reference: [docs/PRD-001-fuseone-agents.md](docs/PRD-001-fuseone-agents.md).

**Scope of this file: the Go core** — `cmd/`, `internal/`, `migrations/`.
Frontend rules are in [web/CLAUDE.md](web/CLAUDE.md) and win inside `web/`.

**Language policy:** everything written down is in English — code, comments,
identifiers, commit messages and the documents in `docs/`. The repository is
public, and a document only half its readers can read is a document that does
not get reviewed. User-facing strings are the exception: they go through i18n,
pt-BR and en-US in parity.

Documents in `docs/` are Markdown, not HTML: GitHub renders them, they diff
line by line in review, and nobody has to generate a PDF to read one.

## Layout

```
cmd/agentd/       Single binary: serve | worker | migrate | verify
internal/
  domain/         Core types. No I/O, stdlib only
  ledger/         Append-only hash-chained run ledger
  gate/           The seven checks and four verdicts
  engine/         Loop interpreter: fold the ledger, decide the next action
  spec/           Versioned agent specifications
  tools/          MCP clients, effect classification
  model/          LLM providers
  trigger/        Cron, webhook, event
  httpapi/        HTTP + SSE
  auth/           OIDC, SAML, delegation
  web/            go:embed of the frontend build
migrations/       Numbered SQL: NNNN_description.sql
web/              React 19 + Vite + shadcn/ui
deploy/helm/      Installation chart
docs/             PRD, ADRs, technical notes
```

---

## Non-negotiable rules

### 1. TDD — the test comes first

Every new behaviour has a test written **before** the implementation. The test
fails first, then passes. If you cannot write the test, you do not yet
understand the requirement.

### 2. The ledger is immutable

There is no `UPDATE` and no `DELETE` on `run_steps` — the database enforces it
with a trigger. Code that tries to amend a recorded step is a bug, never an
optimisation. Corrections are new steps.

### 3. One writer per run (NF-15)

A run has one owner. Serialised writes, monotonic sequence. Never append steps
for the same run from two goroutines.

### 4. No effect bypasses the Gate

No tool call happens outside `gate.Evaluate`. There is no "just this once".

### 5. No secrets in the repository

No `.env`, no tokens, no credentials. Environment variables and the vault.

---

## Size limits

| Unit | Limit | On breach |
|---|---|---|
| Go file | 300 lines | Split by responsibility, never by "type" |
| Function | 40 lines | Extract a pure function |
| Cyclomatic complexity | 10 | Decision table or polymorphism |
| Function parameters | 4 | Options struct |
| Nesting depth | 3 | Guard clauses and early returns |
| React component | 150 lines | Extract a subcomponent |
| Hook | 80 lines | Split into composed hooks |
| Component props | 5 | Composition or a config object |
| Test file | no limit | Tests are documentation |

At ~250 lines in a Go file, **stop and refactor before continuing**.

---

## Go

### Principles

- **`internal/domain` imports nothing but the stdlib.** If a type there needs a
  driver, an HTTP client or an ORM, it is in the wrong package.
- **Dependencies point inward:** `httpapi` → `engine` → `ledger` → `domain`.
  Never the reverse.
- **Interfaces are declared by the consumer, not the implementer.** The
  `Ledger` interface lives next to the `engine` that uses it, not next to the
  Postgres implementation. Go interfaces are structural — the implementation
  never imports the consumer.
- **Accept interfaces, return structs.**
- **`context.Context` is the first parameter** of every function that does I/O.
  Never store a `Context` in a struct.
- **Errors are values.** Wrap with `fmt.Errorf("...: %w", err)`. Sentinels via
  `errors.Is`, typed errors via `errors.As`.
- **Useful zero value.** A freshly declared struct is either usable or fails
  loudly.

### Forbidden

| Forbidden | Use instead |
|---|---|
| `panic` outside `init` or a truly impossible invariant | Return an `error` |
| `any` without a stated reason | Generics or a concrete type |
| Mutable package-level state | Injection through a struct |
| `time.Now()` inside business logic | An injectable `Clock` — otherwise tests are not deterministic |
| `math/rand` on an auditable path | Generate the ID at the edge and record it |
| ORM with automigrate | Explicit SQL in `migrations/` |
| `select` without `case <-ctx.Done()` | Always handle cancellation |
| A goroutine with no owner | Every goroutine has something that stops it |
| Logging tool arguments verbatim | They may carry personal data — log a reference |
| Float for money | Integer micros (`domain.Cost`) |

### Naming

| Item | Convention | Example |
|---|---|---|
| Package | Singular noun, lowercase | `ledger`, `gate` |
| Interface | Noun or `-er` | `Ledger`, `Appender` |
| Constructor | `New<Type>` | `NewMemory` |
| Sentinel error | `Err<Condition>` | `ErrSeqConflict` |
| Test | `Test<Unit>_<condition>_<expectation>` | `TestAppend_sameIdempotencyKeyTwice_secondRejected` |

---

## Tests

- **Test observable behaviour.** Never test a getter; never test that the
  language works.
- **One test, one conceptual assertion.** Several `assert`s about the same
  behaviour are fine; about different behaviours, split them.
- **Table-driven** for variations of one behaviour.
- **`t.Parallel()`** in every test that shares no state.
- **Never mock what you do not own.** For Postgres use a real container or the
  in-memory implementation — never a mock of the driver.
- **The fake enforces the same invariants as the real thing.** A test that
  passes against `ledger.Memory` must not fail in production because the fake
  was more permissive.
- **The name states the scenario.** `TestAppend_seqRewinds_rejected`, not
  `TestAppend2`.
- **Property tests** where they fit: the hash chain and the state fold are the
  natural candidates.

### Not a test

```go
// FORBIDDEN — asserts that Go works
func TestNewRun(t *testing.T) {
    r := NewRun()
    if r == nil { t.Fatal("nil") }
}
```

### A test

```go
// GOOD — asserts a domain invariant
func TestAppend_concurrentWriters_produceContiguousChainWithNoGaps(t *testing.T)
```

---

## Frontend

Frontend rules live in **[web/CLAUDE.md](web/CLAUDE.md)** and are binding for
everything under `web/`. Do not duplicate them here.

---

## Commits

- One commit, one logical change. Never squash the whole task into one.
- Format: `type(scope): description` — `feat(ledger): seal steps into a hash chain`
- Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`
- More than five files touched is probably two commits.

## Definition of done

- [ ] Test written before the implementation, and it failed before it passed
- [ ] `make check` clean — build, vet, lint, test, race
- [ ] No Go file over 300 lines
- [ ] No `any` without a justifying comment
- [ ] `context.Context` threaded through every I/O path
- [ ] Errors wrapped with `%w`, not `%v`
- [ ] No personal data in logs
