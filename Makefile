.DEFAULT_GOAL := check
GO      ?= go
# The console's node_modules can contain vendored Go source (eslint pulls in
# flatted, which ships a Go package). `go test ./...` walks into it, so the
# package list is resolved once and filtered.
PKG     := $(shell $(GO) list ./... | grep -v '/node_modules/')
BIN     := bin/agentd

OAPI    := github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
GEN_GO  := internal/httpapi/openapi/server.gen.go

.PHONY: chart release volume check check-pg test-pg smoke dev stop reset db run-pg build build-api web console test race cover vet fmt lint tidy clean generate verify-generate run

# A database of its own. Sharing one with `make dev` meant a running worker
# claimed the runs a test had just opened, and a test run wiped the
# administrative trail somebody was reading in the console.
TEST_DSN ?= postgres://agents:agents@127.0.0.1:5433/agents_test
# The SQL connector's target: a database standing in for a customer's, with TLS
# and its own roles. Deliberately not the one above — a test that proved the
# executor's guarantees against FuseOne's own store would be proving them where
# the connector must never reach.
TEST_SQL_DSN ?= postgres://sqlconn:sqlconn@127.0.0.1:5434/appx_test?sslmode=verify-full

## check: everything CI runs. Keep this green.
check: fmt vet verify-generate test race console chart

## db: development Postgres. Data lives in tmpfs and is meant to be thrown away.
db:
	docker compose up -d
	@until docker compose exec -T postgres pg_isready -U agents >/dev/null 2>&1; do sleep 1; done
	@docker compose exec -T postgres psql -U agents -d agents -tc \
		"select 1 from pg_database where datname = 'agents_test'" | grep -q 1 || \
		docker compose exec -T postgres createdb -U agents agents_test
	@echo "postgres ready on 5433 (dev: agents, tests: agents_test)"

# The suites that need a real database, asked rather than remembered.
#
# CI used to carry its own copy of this list and had fallen three packages
# behind, so `make check-pg` locally and the postgres job in CI were running
# different suites. Naming it once fixed the disagreement and not the drift:
# the single list then fell a whole package behind on its own, and the suites
# it left out skipped in `check` for want of a DSN and were never reached by
# `check-pg` at all — thirty-nine tests reported green by two targets, neither
# of which had run them.
#
# So it is derived. A suite needs a database exactly when it reads the variable
# that points at one, which is a fact in the files rather than a note somebody
# has to remember to update.
# Two variables, because two databases. TEST_DATABASE_URL is FuseOne's own
# store; TEST_SQL_DATABASE_URL is a database the SQL connector reaches as a
# customer's, with its own TLS, roles and schema. A suite that needed the second
# and found the first would prove the executor's guarantees against the ledger
# it must never touch.
PG_PKGS = $(shell grep -rlE 'TEST_DATABASE_URL|TEST_SQL_DATABASE_URL' --include='*_test.go' internal \
            | xargs -n1 dirname | sort -u | sed 's|^|./|')

## check-pg: the contract suite against a real database as well as the fake.
## A fake that is more permissive than the store is how green tests become
## incidents, so CI runs this, not just `check`.
check-pg: db
	TEST_DATABASE_URL=$(TEST_DSN) TEST_SQL_DATABASE_URL=$(TEST_SQL_DSN) $(MAKE) test-pg

## test-pg: the same suites against whatever TEST_DATABASE_URL and
## TEST_SQL_DATABASE_URL point at. A suite whose database is absent skips and
## says so; it does not fall back to the other one.
## Separate from check-pg because CI brings its own database and must not have
## its own copy of the list.
test-pg:
	@# -p 1: these suites truncate the same tables, and go test runs packages
	@# concurrently by default. Serialising them is the honest fix while they
	@# share one database; a schema per suite would be the next step.
	$(GO) test $(PG_PKGS) -count=1 -race -p 1

## smoke: prove the shipping artefact serves what it should and nothing else.
## The unit suites cannot see this: it is a property of the binary, not the code.
smoke: build
	@BIN=$(BIN) ./scripts/smoke.sh

## dev: the whole platform locally — database, api, worker, console, and
## stand-ins for the model provider and the MCP server. Ctrl-C stops it all.
dev:
	@./scripts/dev.sh

## reset: throw the development database away and start over. The setup token
## is single use, so this is how you get back to a first run.
reset: stop
	docker compose down -v
	@echo "database removed; the next `make dev` starts from a first run"

## stop: end everything `make dev` started, from any terminal.
stop:
	@./scripts/stop.sh

## run-pg: development server backed by Postgres.
run-pg: db build-api
	./$(BIN) serve --demo --dsn $(TEST_DSN) --addr 127.0.0.1:8080

## generate: regenerate everything derived from api/openapi.yaml.
## The spec is the contract; never hand-edit the generated files.
# Both sides of the contract, always together. Generating only Go let the
# console keep a type the server had already stopped speaking, and the gate
# said nothing because it checked one half.
generate:
	cd api && $(GO) run $(OAPI) -config oapi-codegen.yaml openapi.yaml
	cd web && npm run generate --silent
	@echo "generated: $(GEN_GO) $(GEN_TS)"

## verify-generate: fail when committed generated code drifts from the spec.
verify-generate:
	@cp $(GEN_GO) /tmp/agents-gen-before.go
	@cp $(GEN_TS) /tmp/agents-gen-before.ts
	@$(MAKE) --no-print-directory generate >/dev/null
	@if ! diff -q /tmp/agents-gen-before.go $(GEN_GO) >/dev/null; then \
		echo "generated Go is stale — run 'make generate' and commit the result"; \
		cp /tmp/agents-gen-before.go $(GEN_GO); exit 1; \
	fi
	@if ! diff -q /tmp/agents-gen-before.ts $(GEN_TS) >/dev/null; then \
		echo "generated TypeScript is stale — run 'make generate' and commit the result"; \
		cp /tmp/agents-gen-before.ts $(GEN_TS); exit 1; \
	fi

## run: development server with a seeded in-memory ledger.
run: build-api
	./$(BIN) serve --demo --addr 127.0.0.1:8080

## console: the checks CI runs on the frontend.
##
## `tsc --noEmit` at the root checks nothing — tsconfig.json is a references
## file with `files: []`, so it exits clean having compiled zero files. Type
## errors reached main three times before anyone noticed, because the only
## thing that ever ran the real check was CI's `npm run build`.
console:
	cd web && npm run typecheck && npm run lint && npm run test

## web: build the console into the Go package that embeds it.
## go:embed cannot reach outside its own directory, so the output is copied in.
web:
	cd web && npm run build
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	cd web/dist && find . -type f ! -name '*.map' -exec install -Dm644 {} ../../internal/web/dist/{} \;
	cp web/.gitignore-dist internal/web/dist/.gitignore 2>/dev/null || true
	@! find internal/web/dist -name '*.map' | grep -q . || \
		{ echo "refusing to embed a sourcemap"; exit 1; }

## build: the shipping artefact — one binary with the console inside.
build: web
	$(GO) build -tags embedui -o $(BIN) ./cmd/agentd
	@echo "built $(BIN) ($$(du -h $(BIN) | cut -f1))"

## build-api: API only, forwarding the console to a Vite dev server.
build-api:
	$(GO) build -o $(BIN) ./cmd/agentd

test:
	$(GO) test $(PKG) -count=1

race:
	$(GO) test $(PKG) -race -count=1

cover:
	$(GO) test $(PKG) -count=1 -coverprofile=coverage.out
	$(GO) tool cover -func=coverage.out | tail -1

vet:
	$(GO) vet $(PKG)

## fmt: fails instead of rewriting, so CI catches unformatted code.
fmt:
	@out=$$(gofmt -l . ); \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

lint:
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run

tidy:
	$(GO) mod tidy

clean:
	rm -rf bin coverage.out .dev

## volume: a ledger the size a real installation reaches, for measuring plans.
##         STEPS=1000000 make volume
STEPS ?= 1000000
volume: db
	@docker compose exec -T postgres psql -U agents -d agents -tc \
		"select 1 from pg_database where datname = 'agents_vol'" | grep -q 1 || \
		docker compose exec -T postgres createdb -U agents agents_vol
	@docker compose exec -T postgres psql -U agents -d agents_vol -qc \
		"drop schema public cascade; create schema public;" >/dev/null
	@DATABASE_URL=postgres://agents:agents@127.0.0.1:5433/agents_vol \
		$(GO) run ./cmd/agentd migrate
	@docker compose exec -T postgres psql -U agents -d agents_vol -q \
		-v steps=$(STEPS) < scripts/volume.sql

## chart: what renders and still will not run.
## kubeconform proves a manifest has the right shape and not that a volumeMount
## resolves to a volume that exists — which is how a Deployment the API server
## refuses passed CI, lint and review, and reached two published versions.
chart:
	@helm lint deploy/helm/fuseone-agents >/dev/null
	@python3 scripts/chartcheck.py

## release: tag a version, which is what publishes it.
## Nothing is released by merging: the tag is the act, and CI builds the image
## and chart that carry its version, then creates the GitHub Release from the
## changelog. Refuses a dirty tree, an unreleased changelog and a tag that
## already exists — each of the three has shipped somebody a version that does
## not match what they can read.
release:
	@test -n "$(V)" || { echo "usage: make release V=0.2.0"; exit 1; }
	@git diff --quiet || { echo "the tree is dirty; a tag must name a commit somebody can check out"; exit 1; }
	@git rev-parse "v$(V)" >/dev/null 2>&1 && { echo "v$(V) already exists"; exit 1; } || true
	@grep -q "^## \[$(V)\]" CHANGELOG.md || { \
		echo "CHANGELOG.md has no '## [$(V)]' section."; \
		echo "Rename [Unreleased] to [$(V)] and say what an operator has to do before upgrading."; \
		exit 1; }
	@$(MAKE) --no-print-directory check
	git tag -a "v$(V)" -m "$(V)"
	git push origin "v$(V)"
	@echo "tagged. CI publishes ghcr.io/fuseone-io/fuseone-agents:$(V), :latest, the chart, and the GitHub Release"
GEN_TS := web/src/lib/api/schema.gen.ts
