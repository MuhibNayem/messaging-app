# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy only dependency files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the files
COPY . .

# Verify source files exist
RUN test -d cmd/server && test -f cmd/server/main.go || \
    (echo "Source files missing" && exit 1)

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o messaging-app ./cmd/server

# Runtime stage
FROM alpine:3.18

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy only necessary files from builder
COPY --from=builder /app/messaging-app .

# Create non-root user
RUN addgroup -S appgroup && \
    adduser -S appuser -G appgroup && \
    chown appuser:appgroup /app/messaging-app
USER appuser

# Health check
HEALTHCHECK --interval=30s --timeout=3s \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

# Expose ports
EXPOSE 8080
EXPOSE 8181
EXPOSE 9090

# Entrypoint
ENTRYPOINT ["./messaging-app"]