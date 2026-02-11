# =========================
# Stage 1: build
# =========================
FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Install Task for build automation
RUN go install github.com/go-task/task/v3/cmd/task@latest

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and config
COPY . .

# Build with Task (compiles CSS + Go binary)
RUN task build

# Binary is at bin/go-odio-api

# =========================
# Stage 2: runtime
# =========================
FROM debian:bookworm-slim

RUN apt-get update && \
	apt-get install -y --no-install-recommends \
		dbus-x11 \
		ca-certificates && \
	rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary from correct path (Task builds to bin/go-odio-api)
COPY --from=builder /app/bin/go-odio-api /app/odio-api

ENTRYPOINT ["/app/odio-api"]
