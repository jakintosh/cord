all: client server

client:
	go build -o ./bin/cord ./cmd/cord

server:
	go build -o ./bin/cord-server ./cmd/cord-server

test:
	go test ./...
