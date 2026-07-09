# syntax=docker/dockerfile:1
# golang:1.26-alpine
FROM golang:1.26-alpine@sha256:ef18ee7117463ac1055f5a370ed18b8750f01589689d6e6aeae1b3c5a8a8e6c4 AS builder

WORKDIR /build

# Cache deps.
COPY go.mod go.sum ./
RUN go mod download

# Build with version injection.
ARG VERSION=dev
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o /code-reviewer ./cmd/code-reviewer

# Runtime image (~10MB).
# gcr.io/distroless/static-debian12
FROM gcr.io/distroless/static-debian12@sha256:bca175dfe8e21d4e4de21dd8b7eb0f78b32e0a2e7248de06e3b0f7c2dab6b1f5

COPY --from=builder /code-reviewer /usr/local/bin/code-reviewer

ENTRYPOINT ["code-reviewer"]
