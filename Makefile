all: client server

client:
	go build -o ./bin/innernet ./cmd/innernet

server:
	go build -o ./bin/innernet-server ./cmd/innernet-server

test:
	go test ./...
