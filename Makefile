BINARY  := bin/llmtest
CONFIG  ?= config.yaml
FLAGS   ?=

.PHONY: all build test lint check run list clean

all: build

## build: compile the CLI into bin/llmtest
build:
	go build -o $(BINARY) ./cmd/llmtest

## test: run the full offline test suite with the race detector
test:
	go test -race ./...

## lint: the static-analysis gate (format, vet, golangci-lint, gosec)
lint:
	@test -z "$$(gofmt -l . | grep -v '^\.claude/')" || { gofmt -l . | grep -v '^\.claude/'; echo "gofmt: files need formatting"; exit 1; }
	go vet ./...
	golangci-lint run
	gosec -quiet -tests ./...

## check: everything CI would run — build, lint, tests
check: build lint test

## run: execute the benchmark (make run CONFIG=config.yaml FLAGS="--models uni/btl-4")
run: build
	./$(BINARY) run --config $(CONFIG) $(FLAGS)

## list: print the test catalog
list: build
	./$(BINARY) list

clean:
	rm -rf bin
