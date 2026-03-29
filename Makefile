.PHONY: build run dev test migrate docker-build docker-run

# Build the API binary
build:
	go build -o bin/api ./cmd/api

# Run the API
run: build
	./bin/api

# Run with auto-reload (requires air: go install github.com/air-verse/air@latest)
dev:
	air

# Run tests
test:
	go test ./... -v -cover

# Run database migrations (requires psql)
migrate:
	psql $(DATABASE_URL) -f internal/database/migrations/001_initial_schema.sql

# Docker
docker-build:
	docker build -t routewerk .

docker-run:
	docker run --rm -p 8080:8080 --env-file .env routewerk

# Deploy to Fly.io
deploy:
	fly deploy
