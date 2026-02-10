# =========================
# Stage 1: build
# =========================
FROM golang:1.24-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o odio-api

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

COPY --from=builder /app/odio-api /app/odio-api

ENTRYPOINT ["/app/odio-api"]
