# The shipping artefact: one binary with the console inside it (PRD DE-01).
#
# Three stages so the image holds neither a toolchain nor a package manager.
# What ships is a static binary, its certificates, and nothing else — no shell
# to exec into, which is the point rather than an inconvenience: a container
# with a shell is a container somebody debugs in production by changing it.

# The two build stages pin themselves to the machine doing the building rather
# than to the image being built. A console bundle is the same bytes whatever
# runs it, and Go cross-compiles: emulating either through QEMU to reach arm64
# would multiply the build time to produce identical output.

# --- the console ------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:26-alpine AS console

WORKDIR /src/web
# The manifest first, so a change to a component does not reinstall the world.
COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
COPY api/openapi.yaml /src/api/openapi.yaml
RUN npm run build

# --- the binary -------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# The console is copied into the package that embeds it: go:embed cannot reach
# outside its own directory. Sourcemaps are excluded here for the same reason
# the Makefile refuses to embed one — they carry the whole source.
COPY --from=console /src/web/dist/ /tmp/dist/
RUN rm -rf internal/web/dist && mkdir -p internal/web/dist && \
    cd /tmp/dist && find . -type f ! -name '*.map' \
        -exec install -Dm644 {} /src/internal/web/dist/{} \;

# Static, so the runtime image needs no libc. Trimmed, because a path from a
# build machine in a stack trace tells an attacker about the build machine.
ARG VERSION=0.0.0-dev
# Supplied by buildx. Defaulted so a plain `docker build` still works.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -tags embedui -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/agentd ./cmd/agentd

# --- what ships -------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/agentd /agentd

# Non-root by the image's own default, and stated again in the chart so a
# reader of either one knows without checking the other.
USER nonroot:nonroot
EXPOSE 8080

# No CMD: the subcommand is the process's whole identity here — serve, worker,
# migrate — and a default would make the wrong one the easy one.
ENTRYPOINT ["/agentd"]
