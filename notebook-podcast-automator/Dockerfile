# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o automator main.go

# Runtime stage
FROM alpine:latest
WORKDIR /app

# Install certificates for HTTPS
RUN apk --no-cache add ca-certificates

COPY --from=builder /app/automator .

# Default command
CMD ["./automator"]
