.PHONY: all build server client keygen windows-client clean deps test test-race vet integration-test

all: build

deps:
	go mod download
	go mod tidy

build: server client keygen

server:
	@echo "Building server for Linux..."
	go build -o bin/vpn-server ./cmd/server

client:
	@echo "Building client for Linux..."
	go build -o bin/vpn-client ./cmd/client

keygen:
	@echo "Building keygen for Linux..."
	go build -o bin/keygen ./cmd/keygen

windows-client:
	@echo "Building client for Windows..."
	GOOS=windows GOARCH=amd64 go build -o bin/vpn-client.exe ./cmd/client

all-platforms: server client keygen windows-client
	@echo "Building server + keygen for Windows..."
	GOOS=windows GOARCH=amd64 go build -o bin/vpn-server.exe ./cmd/server
	GOOS=windows GOARCH=amd64 go build -o bin/keygen.exe ./cmd/keygen

clean:
	rm -rf bin/
	go clean

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...
	gofmt -l .

# End-to-end test: runs server+client in network namespaces and verifies that
# traffic flows through the encrypted tunnel. Requires root.
integration-test: build
	sudo PATH=$$PATH bash scripts/integration-test.sh

run-server: server
	@echo "Starting server (requires root)..."
	sudo ./bin/vpn-server

run-client: client
	@echo "Starting client (requires root)..."
	sudo ./bin/vpn-client
