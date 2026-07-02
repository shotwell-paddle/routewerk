# ── SPA build stage ────────────────────────────────────────
# Builds the SvelteKit static bundle so the Go stage can embed it via
# //go:embed. Kept separate so changes to Go code don't bust the npm cache.
FROM node:22-alpine AS spa-builder

WORKDIR /spa

COPY web/spa/package.json web/spa/package-lock.json* ./
RUN npm ci --no-audit --no-fund

COPY web/spa/ ./
RUN npm run build

# ── Go build stage ─────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Replace the committed placeholder build with the real SPA bundle so
# //go:embed all:build picks up the production assets.
COPY --from=spa-builder /spa/build/ ./web/spa/build/

RUN CGO_ENABLED=0 GOOS=linux go build -tags=spa_embed -o /api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /admin ./cmd/admin

# ── Runtime stage ──────────────────────────────────────────
# alpine:3.21 (up from 3.19) for postgresql17-client — the staging Fly
# Postgres runs 17.7 and pg_dump refuses servers NEWER than itself.
# pg_dump 17 also dumps older servers, so it covers a PG 16 prod too.
FROM alpine:3.21

# postgresql17-client provides pg_dump/pg_restore for the in-app nightly
# backup (service/backup.go). Client major must be >= the server major;
# bump the client when the Fly Postgres is upgraded.
RUN apk add --no-cache ca-certificates tzdata postgresql17-client \
    && addgroup -S routewerk && adduser -S routewerk -G routewerk

WORKDIR /app

COPY --from=builder /api .
COPY --from=builder /admin .
COPY --from=builder /app/internal/database/migrations ./migrations

# Run as non-root
USER routewerk

EXPOSE 8080

CMD ["./api"]
