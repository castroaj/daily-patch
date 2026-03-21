# Makefile — build targets for the api service
#
# Targets: build, test, lint, run, docker-build, clean

.PHONY: build test lint run docker-build clean

build:
	go build -o bin/api .

test:
	go test ./...

lint:
	golangci-lint run

run:
	go run .

docker-build:
	docker build -t daily-patch/api .

clean:
	rm -rf bin/
