# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Cache deps.
COPY go.mod go.sum ./
RUN go mod download

# Build.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /code-reviewer ./cmd/code-reviewer

# Runtime image (~10MB).
FROM gcr.io/distroless/static-debian12

COPY --from=builder /code-reviewer /usr/local/bin/code-reviewer

ENTRYPOINT ["code-reviewer"]
