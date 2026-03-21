.PHONY: build test lint up down clean

build:
	$(MAKE) -C api build
	$(MAKE) -C ingestion build

test:
	$(MAKE) -C api test
	$(MAKE) -C ingestion test
	$(MAKE) -C scorer test
	$(MAKE) -C generator test

lint:
	$(MAKE) -C api lint
	$(MAKE) -C ingestion lint

up:
	docker compose up

down:
	docker compose down

clean:
	$(MAKE) -C api clean
	$(MAKE) -C ingestion clean
	$(MAKE) -C scorer clean
	$(MAKE) -C generator clean
