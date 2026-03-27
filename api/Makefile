# Makefile — build targets for the api service
#
# Targets: build, test, coverage, lint, run, docker-build, clean, setup

# Prefer gobrew-managed Go when available; fall back to PATH go.
GOBREW_GO     := $(HOME)/.gobrew/current/bin/go
GO            := $(if $(wildcard $(GOBREW_GO)),$(GOBREW_GO),go)

GOLANGCI_LINT_VERSION := v1.64.8
GOLANGCI_LINT_BIN     := $(shell $(GO) env GOPATH)/bin/golangci-lint

.PHONY: build test coverage lint run docker-build clean setup

build:
	$(GO) build -o bin/api .

test:
	$(GO) test ./... -v -cover

coverage:
	$(GO) test ./... -v -coverprofile=coverage.out; $(GO) tool cover -func=coverage.out

# Lints the service; installs golangci-lint automatically if not present.
# PATH is prefixed with gobrew's bin so golangci-lint resolves the correct go.
lint:
	PATH=$(dir $(GO)):$$PATH $(GOLANGCI_LINT_BIN) run

run:
	$(GO) run . -c $(PWD)/config.local.yaml

docker-build:
	docker build -t daily-patch/api .

clean:
	rm -rf bin/
	rm -f ./coverage.out

# Explicitly install golangci-lint at the pinned version.
setup: $(GOLANGCI_LINT_BIN)

$(GOLANGCI_LINT_BIN):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b $(shell $(GO) env GOPATH)/bin $(GOLANGCI_LINT_VERSION)
