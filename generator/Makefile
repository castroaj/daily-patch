.PHONY: install run test docker-build clean

install:
	pip install -r requirements.txt

run:
	python3 -m generator

test:
	pytest

docker-build:
	docker build -t daily-patch/generator .

clean:
	find . -type d -name __pycache__ -exec rm -rf {} +
	find . -type d -name .pytest_cache -exec rm -rf {} +
