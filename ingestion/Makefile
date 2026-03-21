# Makefile — build targets for the ingestion service
#
# Targets: build, test, lint, run, docker-build, clean

.PHONY: build test lint run docker-build clean

build:
	go build -o bin/ingestion .

test:
	go test ./...

lint:
	golangci-lint run

run:
	go run .

docker-build:
	docker build -t daily-patch/ingestion .

clean:
	rm -rf bin/
