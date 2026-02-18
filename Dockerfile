# =========================
# Stage 1: build
# =========================
FROM golang:1.24-bookworm AS builder

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and config
COPY . .

# Cross-compile using TARGETOS/TARGETARCH/TARGETVARIANT passed by docker buildx.
# CGO_ENABLED=0: pure Go, no cgo needed (dbus/systemd/pulseaudio via godbus).
# CSS is embedded at compile time and must be present; use pre-built output.css
# if available, otherwise the build will fail (run task css first or ensure
# ui/static/output.css is committed / available from CDN).
RUN GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    GOARM=$(echo "${TARGETVARIANT}" | sed 's/^v//') \
    CGO_ENABLED=0 \
    go build \
      -ldflags="-X 'github.com/b0bbywan/go-odio-api/config.AppVersion=${VERSION}'" \
      -o bin/odio-api .

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

COPY --from=builder /app/bin/odio-api /app/odio-api

ENTRYPOINT ["/app/odio-api"]
