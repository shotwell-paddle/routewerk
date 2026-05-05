.PHONY: build build-admin run dev test migrate migrate-down migrate-version docker-build docker-run refresh-dev-db spa-install spa-build spa-dev spa-check api-gen api-gen-check

# Build the API binary with the embedded SPA. Requires Node + npm.
# The spa_embed build tag flips embed.go on; without it, embed_stub.go
# returns 503 so plain `go build ./...` works on a fresh checkout.
build: spa-build
	go build -tags=spa_embed -o bin/api ./cmd/api

# Build the admin CLI
build-admin:
	go build -o bin/admin ./cmd/admin

# Run the API (auto-migrates on startup)
run: build
	./bin/api

# Run with auto-reload (requires air: go install github.com/air-verse/air@latest)
dev:
	air

# Run tests with the SPA embedded so embed_test.go runs against the real
# bundle rather than the stub. spa-build is a prerequisite.
test: spa-build
	go test -tags=spa_embed ./... -v -cover

# Database migrations via the admin CLI
migrate: build-admin
	./bin/admin migrate

migrate-down: build-admin
	./bin/admin migrate-down

migrate-version: build-admin
	./bin/admin migrate-version

# Docker
docker-build:
	docker build -t routewerk .

docker-run:
	docker run --rm -p 8080:8080 --env-file .env routewerk

# Deploy to Fly.io
deploy:
	fly deploy

# ── SPA (SvelteKit) ────────────────────────────────────────
# The SPA lives in web/spa/ and is embedded into the Go binary at compile
# time. `make build` runs spa-build first; if you want to rebuild only the
# Go side use `go build` directly (the embedded files won't refresh).

spa-install:
	cd web/spa && npm ci

spa-build:
	cd web/spa && npm run build

spa-dev:
	cd web/spa && npm run dev

spa-check:
	cd web/spa && npm run check

# ── OpenAPI ────────────────────────────────────────────────
# Source of truth: api/openapi.yaml. Regenerates Go server interfaces
# (internal/api/api.gen.go via oapi-codegen, tracked as a Go tool) and
# the SvelteKit client types (web/spa/src/lib/api/types.ts via
# openapi-typescript, tracked as a npm devDependency).
#
# Generated files are committed; CI regenerates and checks for drift —
# see api-gen-check below and .github/workflows/ci.yml.
api-gen:
	go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
	cd web/spa && npm run api-gen

# Regen and fail if anything changed. Used by CI to catch spec/code drift
# (someone edited openapi.yaml without running `make api-gen`, or
# hand-edited a generated file). Run locally before pushing if you want
# to mirror the CI check.
api-gen-check: api-gen
	@if ! git diff --quiet -- internal/api/ web/spa/src/lib/api/types.ts; then \
		echo "ERROR: generated files are out of sync with api/openapi.yaml"; \
		echo "Run 'make api-gen' and commit the result."; \
		git --no-pager diff --stat -- internal/api/ web/spa/src/lib/api/types.ts; \
		exit 1; \
	fi
	@echo "api-gen drift check: OK"

# Refresh dev database from production
# Requires two proxies running in separate terminals first:
#   Terminal 1: fly proxy 15432:5432 -a routewerk-db
#   Terminal 2: fly proxy 15433:5432 -a routewerk-dev-db
refresh-dev-db:
	@echo "Dumping production via proxy on localhost:15432..."
	pg_dump --no-owner --no-acl -Fc -h localhost -p 15432 -U routewerk -d routewerk -f /tmp/routewerk_prod.dump
	@echo "Restoring to dev via proxy on localhost:15433..."
	pg_restore --clean --no-owner --no-acl -h localhost -p 15433 -U postgres -d routewerk_dev /tmp/routewerk_prod.dump
	@echo "Done. Dev database refreshed from production."
