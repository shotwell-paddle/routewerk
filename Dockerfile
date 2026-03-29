# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /admin ./cmd/admin

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S routewerk && adduser -S routewerk -G routewerk

WORKDIR /app

COPY --from=builder /api .
COPY --from=builder /admin .
COPY --from=builder /app/internal/database/migrations ./migrations

# Run as non-root
USER routewerk

EXPOSE 8080

CMD ["./api"]
