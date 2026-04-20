# Multi-stage build for Infrastructure API service

# Stage 1: Build the Go binary
FROM golang:1.25.3-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -o infrastructure-api .

# Stage 2: Runtime image
FROM alpine:3.19

# Install runtime dependencies (ca-certificates for HTTPS, tzdata for timestamps)
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for security
RUN addgroup -g 1000 vortex && \
    adduser -D -u 1000 -G vortex vortex

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/infrastructure-api .

# Change ownership to non-root user
RUN chown -R vortex:vortex /app

# Switch to non-root user
USER vortex

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./infrastructure-api"]
