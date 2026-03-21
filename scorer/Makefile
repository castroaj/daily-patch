# Makefile — build targets for the scorer service
#
# Targets: setup, run, test, lint, docker-build, clean

VENV := .venv
PYTHON := $(VENV)/bin/python
PIP := $(VENV)/bin/pip
PYTEST := $(VENV)/bin/pytest
RUFF := $(VENV)/bin/ruff

.PHONY: setup run test lint docker-build clean

setup: $(VENV)/bin/activate

$(VENV)/bin/activate: requirements.txt
	python3 -m venv $(VENV)
	$(PIP) install -r requirements.txt
	touch $(VENV)/bin/activate

run: setup
	$(PYTHON) -m scorer

test: setup
	$(PYTEST)

lint: setup
	$(RUFF) check .
	$(RUFF) format --check .

docker-build:
	docker build -t daily-patch/scorer .

clean:
	rm -rf $(VENV)
	find . -type d -name __pycache__ -exec rm -rf {} +
	find . -type d -name .pytest_cache -exec rm -rf {} +
