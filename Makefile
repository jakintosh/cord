GO := go
BIN_DIR := ./bin
APP := cord
BIN := $(BIN_DIR)/$(APP)
CMD := ./cmd/$(APP)

.DEFAULT_GOAL := help

.PHONY: help all build generate test test-full fmt vet lint install run clean

help:
	@printf "Targets:\n"
	@printf "  build      Build $(BIN)\n"
	@printf "  test       Run tests\n"
	@printf "  test-full  Run full test suite, including '-race'\n"
	@printf "  lint       Format and vet Go code\n"
	@printf "  install    Install the cord CLI binary\n"
	@printf "  run        Run the cord daemon\n"
	@printf "  clean      Remove build artifacts\n"

all: build

build: generate
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN) $(CMD)

generate:
	$(GO) generate ./...

test: generate
	$(GO) test ./...

test-full:
	$(GO) test -race ./...

fmt:
	$(GO) fmt ./...

vet: generate
	$(GO) vet ./...

lint: fmt vet

install: generate
	GOBIN=/usr/local/bin $(GO) install $(CMD)

run: build
	$(BIN) daemon

clean:
	rm -rf $(BIN_DIR)
