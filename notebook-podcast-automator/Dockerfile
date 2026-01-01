# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Ensure VCS + TLS works for `go mod download` in Alpine
RUN apk --no-cache add git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o automator .

# Runtime stage
FROM alpine:latest
WORKDIR /app

# Install certificates + tzdata for HTTPS and timezone scheduling
RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/automator .

EXPOSE 8080

# Default command
CMD ["./automator"]
