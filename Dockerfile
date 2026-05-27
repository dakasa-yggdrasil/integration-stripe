# syntax=docker/dockerfile:1.7
# Build context MUST be the integration-stripe repo root.

FROM golang:1.25-bookworm AS build

WORKDIR /build

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GOPROXY=https://proxy.golang.org,direct

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    sh -c 'for attempt in 1 2 3; do go mod download && exit 0; sleep 5; done; exit 1'

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /bin/integration-stripe ./cmd/adapter

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

COPY --from=build /bin/integration-stripe /app/integration-stripe

# 8080 health, 8081 RPC, 8082 webhook
EXPOSE 8080 8081 8082
ENTRYPOINT ["/app/integration-stripe"]
