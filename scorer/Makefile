# Makefile — build targets for the scorer service
#
# Targets: install, run, test, docker-build, clean

.PHONY: install run test docker-build clean

install:
	pip install -r requirements.txt

run:
	python3 -m scorer

test:
	pytest

docker-build:
	docker build -t daily-patch/scorer .

clean:
	find . -type d -name __pycache__ -exec rm -rf {} +
	find . -type d -name .pytest_cache -exec rm -rf {} +
