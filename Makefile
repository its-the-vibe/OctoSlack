BINARY_NAME := octoslack
GO_FILES    := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: all build test lint fmt clean

all: build

## build: compile the binary
build:
	CGO_ENABLED=0 go build -o $(BINARY_NAME) .

## test: run all unit tests
test:
	go test -v -race ./...

## lint: run go vet (no external tools required)
lint:
	go vet ./...

## fmt: check formatting
fmt:
	@if [ -n "$$(gofmt -l $(GO_FILES))" ]; then \
		echo "The following files are not formatted:"; \
		gofmt -l $(GO_FILES); \
		exit 1; \
	fi

## clean: remove build artifacts
clean:
	rm -f $(BINARY_NAME)
