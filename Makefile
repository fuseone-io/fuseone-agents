.DEFAULT_GOAL := check
GO      ?= go
# The console's node_modules can contain vendored Go source (eslint pulls in
# flatted, which ships a Go package). `go test ./...` walks into it, so the
# package list is resolved once and filtered.
PKG     := $(shell $(GO) list ./... | grep -v '/node_modules/')
BIN     := bin/agentd

OAPI    := github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
GEN_GO  := internal/httpapi/openapi/server.gen.go

.PHONY: volume check check-pg test-pg smoke dev stop reset db run-pg build build-api web console test race cover vet fmt lint tidy clean docs generate verify-generate run

# A database of its own. Sharing one with `make dev` meant a running worker
# claimed the runs a test had just opened, and a test run wiped the
# administrative trail somebody was reading in the console.
TEST_DSN ?= postgres://agents:agents@127.0.0.1:5433/agents_test

## check: everything CI runs. Keep this green.
check: fmt vet verify-generate test race console

## db: development Postgres. Data lives in tmpfs and is meant to be thrown away.
db:
	docker compose up -d
	@until docker compose exec -T postgres pg_isready -U agents >/dev/null 2>&1; do sleep 1; done
	@docker compose exec -T postgres psql -U agents -d agents -tc \
		"select 1 from pg_database where datname = 'agents_test'" | grep -q 1 || \
		docker compose exec -T postgres createdb -U agents agents_test
	@echo "postgres ready on 5433 (dev: agents, tests: agents_test)"

# The suites that need a real database, named once.
#
# CI used to carry its own copy of this list and had fallen three packages
# behind, so `make check-pg` locally and the postgres job in CI were running
# different suites — and the ones CI was missing were the newest.
PG_PKGS = ./internal/ledger/ ./internal/e2e/ ./internal/admin/ ./internal/spec/ \
          ./internal/auth/ ./internal/trigger/ ./internal/audit/ ./internal/policy/ \
          ./internal/scope/ ./internal/authoring/ ./internal/regression/

## check-pg: the contract suite against a real database as well as the fake.
## A fake that is more permissive than the store is how green tests become
## incidents, so CI runs this, not just `check`.
check-pg: db
	TEST_DATABASE_URL=$(TEST_DSN) $(MAKE) test-pg

## test-pg: the same suites against whatever TEST_DATABASE_URL points at.
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
generate:
	cd api && $(GO) run $(OAPI) -config oapi-codegen.yaml openapi.yaml
	@echo "generated: $(GEN_GO)"

## verify-generate: fail when committed generated code drifts from the spec.
verify-generate:
	@cp $(GEN_GO) /tmp/agents-gen-before.go
	@$(MAKE) --no-print-directory generate >/dev/null
	@if ! diff -q /tmp/agents-gen-before.go $(GEN_GO) >/dev/null; then \
		echo "generated code is stale — run 'make generate' and commit the result"; \
		cp /tmp/agents-gen-before.go $(GEN_GO); exit 1; \
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

docs:
	$(MAKE) -C docs pdf

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
