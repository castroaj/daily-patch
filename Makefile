# Root Makefile — daily-patch
#
# Delegates to each service subdirectory. Go services (api, ingestion) support
# build/test/lint/clean; Python services (scorer, generator) support
# install/run/test/clean. Docker Compose targets manage the full stack.

.PHONY: build test lint up down clean

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

# Lint all Go services (requires golangci-lint)
lint:
	$(MAKE) -C api lint
	$(MAKE) -C ingestion lint

# Start the full Docker Compose stack
up:
	docker compose up

# Stop and remove containers
down:
	docker compose down

# Remove all build artifacts and caches across every service
clean:
	$(MAKE) -C api clean
	$(MAKE) -C ingestion clean
	$(MAKE) -C scorer clean
	$(MAKE) -C generator clean
