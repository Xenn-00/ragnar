# ============================================================
# Stage 1: Builder
# Compile the binaries (server + worker) here
# ============================================================
FROM golang:1.25-alpine AS builder

# Install build dependencies for CGO (needed by leedongthuc/pdf)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy dependency files - this layer gonna be cached as long as go.mod/go.sum doesn't change
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build server binary
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /bin/server \
    ./cmd/server

# Build worker binary
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /bin/worker \
    ./cmd/worker

# ============================================================
# Stage 2a: Server runtime image
# Minimal image — only binary + ca-certificates
# ============================================================
FROM alpine:3.20 AS server

# ca-certificates needed for HTTPS calls to Ollama
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/server .

EXPOSE 8080

CMD [ "./server" ]

# ============================================================
# Stage 2b: Server runtime image
# ============================================================
FROM alpine:3.20 AS worker

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/worker .

CMD [ "./worker" ]