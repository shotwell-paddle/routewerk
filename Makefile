.PHONY: build build-admin run dev test migrate migrate-down migrate-version docker-build docker-run refresh-dev-db

# Build the API binary
build:
	go build -o bin/api ./cmd/api

# Build the admin CLI
build-admin:
	go build -o bin/admin ./cmd/admin

# Run the API (auto-migrates on startup)
run: build
	./bin/api

# Run with auto-reload (requires air: go install github.com/air-verse/air@latest)
dev:
	air

# Run tests
test:
	go test ./... -v -cover

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
