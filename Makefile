.PHONY: all build test test-python probe-image clean

GO ?= go
PYTHON ?= python3

all: test build

build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/dlbench ./cmd/dlbench

test:
	$(GO) test ./...

test-python:
	PYTHONPATH=python/src $(PYTHON) -m unittest discover -s python/tests -v

probe-image:
	docker build --platform linux/amd64 -t dlbench-probe:smoke probe

clean:
	rm -rf bin dist

