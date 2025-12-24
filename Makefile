.PHONY: all build clean test proto docker help

# Variables
BINARY_NAME=savalet
VERSION=$(shell git describe --tags --always --dirty)
LDFLAGS=-ldflags "-w -s -X 'github.com/zinrai/savalet/cmd.version=$(VERSION)'"
PROTO_FILES=savalet.proto

# Default target
all: proto build

# Help target
help:
	@echo "Available targets:"
	@echo "  build       - Build the binary"
	@echo "  proto       - Generate protobuf files"
	@echo "  test        - Run tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  docker      - Build Docker image"
	@echo "  run-daemon  - Run daemon mode locally"
	@echo "  run-api     - Run API mode locally"

# Generate protobuf files
proto:
	mkdir -p pb
	protoc --go_out=pb --go_opt=paths=source_relative \
		--go-grpc_out=pb --go-grpc_opt=paths=source_relative \
		$(PROTO_FILES)

# Build the binary
build: proto
	go build $(LDFLAGS) -o $(BINARY_NAME) .

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)

# Build Docker image
docker:
	docker build -t savalet:latest .

# Development targets
run-daemon:
	go run . daemon --config configs/daemon.yaml --log-level debug

run-api:
	go run . api --config configs/api.yaml --log-level debug

# Format code
fmt:
	go fmt ./...
	gofmt -s -w .
