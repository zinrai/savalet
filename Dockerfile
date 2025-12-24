# Build stage
FROM golang:1.24-alpine3.22 AS builder

# Install build dependencies
RUN apk add --no-cache git make protoc protobuf-dev

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
    -ldflags="-w -s -X 'github.com/zinrai/savalet/cmd.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)'" \
    -o savalet .

# Runtime stage
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata && \
    rm -rf /var/cache/apk/*

# Create non-root user and group
RUN addgroup -g 1000 -S savalet && \
    adduser -u 1000 -S savalet -G savalet

# Create necessary directories
RUN mkdir -p /etc/savalet /var/run && \
    chown savalet:savalet /var/run

# Copy binary from builder
COPY --from=builder /build/savalet /usr/local/bin/savalet
RUN chmod 755 /usr/local/bin/savalet

# Copy default API configuration
COPY --chown=savalet:savalet configs/api.yaml /etc/savalet/api.yaml

# Switch to non-root user
USER savalet

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set default command for API mode
ENTRYPOINT ["/usr/local/bin/savalet"]
CMD ["api", "--config", "/etc/savalet/api.yaml"]
