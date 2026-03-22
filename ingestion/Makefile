# Makefile — build targets for the ingestion service
#
# Targets: build, test, lint, run, docker-build, clean, setup

GOLANGCI_LINT_VERSION := v1.64.8
GOLANGCI_LINT_BIN     := $(shell go env GOPATH)/bin/golangci-lint

.PHONY: build test lint run docker-build clean setup

build:
	go build -o bin/ingestion .

test:
	go test ./... -v

# Lints the service; installs golangci-lint automatically if not present.
lint:
	$(GOLANGCI_LINT_BIN) run

run:
	go run .

docker-build:
	docker build -t daily-patch/ingestion .

clean:
	rm -rf bin/

# Explicitly install golangci-lint at the pinned version.
setup: $(GOLANGCI_LINT_BIN)

$(GOLANGCI_LINT_BIN):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)
