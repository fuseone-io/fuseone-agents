# The stand-in tool servers, as an image.
#
# A separate image from the platform, and deliberately: a fixture that shipped
# inside the product could be started by accident in a customer's installation,
# and a CRM that answers every lookup with the same invented customer is a
# convincing thing to find running somewhere real.
#
# Same base and same posture as the platform's own — static, distroless, no
# shell — because a lab that is easier to break into than production teaches
# habits nobody wants back.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=0.0.0-dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w" -o /out/devstack ./cmd/devstack

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/devstack /devstack
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/devstack", "mcp"]
