GO := go
BIN_DIR := ./bin
APP := cord
BIN := $(BIN_DIR)/$(APP)
CMD := ./cmd/$(APP)

.DEFAULT_GOAL := help

.PHONY: help all build generate test test-integration fmt vet lint install clean

help:
	@printf "Targets:\n"
	@printf "  build             Build $(BIN)\n"
	@printf "  test              Run unit tests\n"
	@printf "  test-integration  Run privileged WireGuard integration tests\n"
	@printf "  lint              Format and vet Go code\n"
	@printf "  install           Install the cord CLI binary\n"
	@printf "  clean             Remove build artifacts\n"

all: build

build: generate
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN) $(CMD)

generate:
	$(GO) generate ./...

test: generate
	$(GO) test ./...

# creates real WireGuard interfaces; run as root: `sudo make test-integration`
test-integration:
	$(GO) test -tags integration -count=1 -v -run Integration ./internal/wireguard

fmt:
	$(GO) fmt ./...

vet: generate
	$(GO) vet ./...

lint: fmt vet

install: generate
	$(GO) install $(CMD)

clean:
	rm -rf $(BIN_DIR)
