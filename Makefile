# Root Makefile — daily-patch
#
# Delegates to each service subdirectory. Go services (api, ingestion) support
# build/test/lint/docker-build/clean; Python services (scorer, generator) support
# setup/run/test/lint/docker-build/clean. Docker Compose targets manage the full stack.

.PHONY: build test lint docker-build up down clean setup

# Build all Go service binaries
build:
	$(MAKE) -C api build
	$(MAKE) -C ingestion build

# Run tests across all services
test:
	$(MAKE) -C api test
	$(MAKE) -C ingestion test
	$(MAKE) -C scorer test
	$(MAKE) -C generator test

# Lint all services; golangci-lint for Go, ruff for Python
lint:
	$(MAKE) -C api lint
	$(MAKE) -C ingestion lint
	$(MAKE) -C scorer lint
	$(MAKE) -C generator lint

# Build Docker images for all services
docker-build:
	$(MAKE) -C api docker-build
	$(MAKE) -C ingestion docker-build
	$(MAKE) -C scorer docker-build
	$(MAKE) -C generator docker-build

# Start the full Docker Compose stack
up:
	docker compose up

# Stop and remove containers
down:
	docker compose down

# Install dev tools (golangci-lint) for all Go services
setup:
	$(MAKE) -C api setup
	$(MAKE) -C ingestion setup

# Remove all build artifacts and caches across every service
clean:
	$(MAKE) -C api clean
	$(MAKE) -C ingestion clean
	$(MAKE) -C scorer clean
	$(MAKE) -C generator clean
