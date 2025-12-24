# Build stage
FROM golang:1.24-alpine3.22 AS builder

# Install build dependencies
RUN apk add --no-cache git protoc protobuf-dev

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy proto file and generate code
COPY savalet.proto ./
RUN mkdir -p pb && \
    protoc --go_out=pb --go_opt=paths=source_relative \
           --go-grpc_out=pb --go-grpc_opt=paths=source_relative \
           savalet.proto

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -a -installsuffix cgo \
    -ldflags="-w -s -X 'main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)'" \
    -o savalet .

# Runtime stage
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates wget && \
    rm -rf /var/lib/apt/lists/*

# Create non-root user and group
RUN groupadd -g 1000 savalet && \
    useradd -u 1000 -g savalet -s /sbin/nologin savalet

# Create necessry directories
RUN mkdir -p /etc/savalet /var/run && \
    chown savalet:savalet /var/run

# Copy binary from builder
COPY --from=builder /build/savalet /usr/local/bin/savalet
RUN chmod 755 /usr/local/bin/savalet

# Copy default API configuration
COPY --chown=savalet:savalet example/api.yaml /etc/savalet/api.yaml

# Switch to non-root user
USER savalet

# Expose HTTP port
EXPOSE 9090

# Set default command for API mode
ENTRYPOINT ["/usr/local/bin/savalet"]
CMD ["api", "-config", "/etc/savalet/api.yaml"]
