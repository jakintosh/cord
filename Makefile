all: client server

client: generate
	go build -o ./bin/cord ./cmd/cord

server: generate
	go build -o ./bin/cord-server ./cmd/cord-server

generate:
	go generate ./...

test:
	go test ./...

# creates real WireGuard interfaces; run as root: `sudo make test-integration`
test-integration:
	go test -tags integration -count=1 -v -run Integration ./internal/wireguard

clean:
	rm -rf ./bin

.PHONY: all client server generate test test-integration clean
