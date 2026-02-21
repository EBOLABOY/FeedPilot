# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Ensure VCS + TLS works for `go mod download` in Alpine
RUN apk --no-cache add git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -buildvcs=false -o automator .

# Runtime stage
FROM alpine:latest
WORKDIR /app

# Install runtime deps:
# - ca-certificates/tzdata for network + scheduling
# - chromium for interactive re-auth fallback
# - xvfb/fluxbox/x11vnc for optional containerized browser UI
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    chromium \
    xvfb \
    fluxbox \
    x11vnc \
    dumb-init

COPY --from=builder /app/automator .
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 8080
EXPOSE 5900

# Default command
ENTRYPOINT ["dumb-init", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["./automator"]
