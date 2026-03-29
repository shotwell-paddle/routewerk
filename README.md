# Routewerk

Route setting management platform for climbing gyms.

## Stack

- **API**: Go + Chi router
- **Database**: PostgreSQL
- **Web Dashboard**: Go + Templ + HTMX
- **Mobile**: Flutter (Dart)
- **Hosting**: Fly.io

## Getting Started

### Prerequisites

- Go 1.22+
- PostgreSQL 16+
- Make

### Setup

```bash
# Copy environment config
cp .env.example .env
# Edit .env with your database credentials

# Run database migrations
make migrate

# Start the server
make run

# Or with live reload (requires air)
make dev
```

### Project Structure

```
cmd/
  api/              # API server entrypoint
  worker/           # Background worker entrypoint
internal/
  auth/             # JWT and password handling
  config/           # Environment config
  database/         # DB connection and migrations
  handler/          # HTTP handlers
  middleware/       # Auth, logging, etc.
  model/            # Data models
  repository/       # Database queries
  router/           # Route definitions
  service/          # Business logic
web/
  templates/        # Templ templates for dashboard
  static/           # Static assets
deploy/             # Deployment scripts
```
