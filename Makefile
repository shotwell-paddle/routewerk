.PHONY: build build-admin run dev test migrate migrate-down migrate-version docker-build docker-run

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
