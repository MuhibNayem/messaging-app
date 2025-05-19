# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN test -d cmd/server && test -f cmd/server/main.go || \
    (echo "Source files missing" && exit 1)

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o messaging-app ./cmd/server

# Runtime stage
FROM alpine:3.18

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/messaging-app .

RUN addgroup -S appgroup && \
    adduser -S appuser -G appgroup && \
    chown appuser:appgroup /app/messaging-app
USER appuser

HEALTHCHECK --interval=30s --timeout=3s \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

EXPOSE 8080
EXPOSE 8181
EXPOSE 9090

ENTRYPOINT ["./messaging-app"]