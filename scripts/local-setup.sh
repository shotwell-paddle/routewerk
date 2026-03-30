#!/bin/bash
set -e

echo "=== Routewerk Local Setup ==="
echo ""

# Check for Homebrew
if ! command -v brew &> /dev/null; then
    echo "❌ Homebrew not found. Install it first:"
    echo '   /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'
    exit 1
fi
echo "✅ Homebrew found"

# Install PostgreSQL 16
if brew list postgresql@16 &> /dev/null; then
    echo "✅ PostgreSQL 16 already installed"
else
    echo "📦 Installing PostgreSQL 16..."
    brew install postgresql@16
fi

# Start PostgreSQL
if brew services list | grep postgresql@16 | grep started &> /dev/null; then
    echo "✅ PostgreSQL 16 already running"
else
    echo "🚀 Starting PostgreSQL 16..."
    brew services start postgresql@16
fi

# Wait a moment for Postgres to be ready
sleep 2

# Create database and user
echo ""
echo "📦 Setting up database..."

if psql -lqt | cut -d \| -f 1 | grep -qw routewerk; then
    echo "✅ Database 'routewerk' already exists"
else
    createdb routewerk
    echo "✅ Created database 'routewerk'"
fi

# Create user (ignore error if already exists)
psql -d routewerk -c "
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'routewerk') THEN
        CREATE ROLE routewerk WITH LOGIN PASSWORD 'password';
    END IF;
END
\$\$;
" 2>/dev/null
echo "✅ User 'routewerk' ready"

# Grant privileges
psql -d routewerk -c "GRANT ALL PRIVILEGES ON DATABASE routewerk TO routewerk;" 2>/dev/null
psql -d routewerk -c "GRANT ALL PRIVILEGES ON SCHEMA public TO routewerk;" 2>/dev/null
echo "✅ Privileges granted"

# Create .env if it doesn't exist
if [ ! -f .env ]; then
    cp .env.example .env
    echo "✅ Created .env from .env.example"
else
    echo "✅ .env already exists"
fi

# Fetch Go dependencies
echo ""
echo "📦 Fetching Go dependencies..."
go mod tidy
echo "✅ Dependencies fetched"

# Verify build
echo ""
echo "🔨 Verifying build..."
go build ./...
echo "✅ Build successful"

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Run the app with:  make run"
echo "It will auto-migrate the database on startup."
echo ""
