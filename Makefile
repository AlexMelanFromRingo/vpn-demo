.PHONY: all build server client windows-client clean deps test

all: build

deps:
	go mod download
	go mod tidy

build: server client

server:
	@echo "Building server for Linux..."
	go build -o bin/vpn-server ./cmd/server

client:
	@echo "Building client for Linux..."
	go build -o bin/vpn-client ./cmd/client

windows-client:
	@echo "Building client for Windows..."
	GOOS=windows GOARCH=amd64 go build -o bin/vpn-client.exe ./cmd/client

all-platforms: server client windows-client
	@echo "Building server for Windows..."
	GOOS=windows GOARCH=amd64 go build -o bin/vpn-server.exe ./cmd/server

clean:
	rm -rf bin/
	go clean

test:
	go test -v ./...

run-server: server
	@echo "Starting server (requires root)..."
	sudo ./bin/vpn-server

run-client: client
	@echo "Starting client (requires root)..."
	sudo ./bin/vpn-client
