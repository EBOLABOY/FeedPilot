# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
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
