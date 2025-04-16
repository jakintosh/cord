build-client:
	go build -o ./bin/innernet ./cmd/innernet

build-server:
	go build -o ./bin/innernet-server ./cmd/innernet-server

all: build-client build-server
