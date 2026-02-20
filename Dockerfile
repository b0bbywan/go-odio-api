# =========================
# Stage 1: build
# =========================
FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS builder

ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG BUILDARCH
ARG TAILWIND_VERSION=v3.4.17

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and config
COPY . .

# Build CSS using Tailwind CLI (runs natively on build platform, not target platform).
# The generated output.css is embedded in the Go binary via go:embed.
RUN case "${BUILDARCH}" in \
      amd64) TW_BIN="tailwindcss-linux-x64" ;; \
      arm64) TW_BIN="tailwindcss-linux-arm64" ;; \
      arm)   TW_BIN="tailwindcss-linux-armv7" ;; \
      *)     echo "Unsupported BUILDARCH: ${BUILDARCH}"; exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/${TW_BIN}" \
      -o /usr/local/bin/tailwindcss && \
    chmod +x /usr/local/bin/tailwindcss && \
    tailwindcss -i ui/styles/input.css -o ui/static/output.css --minify

# Cross-compile using TARGETOS/TARGETARCH/TARGETVARIANT passed by docker buildx.
# CGO_ENABLED=0: pure Go, no cgo needed (dbus/systemd/pulseaudio via godbus).
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
