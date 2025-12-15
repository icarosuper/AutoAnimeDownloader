# Stage 1: Build Go daemon
FROM golang:1.24-alpine AS go-builder

WORKDIR /build

# Copy go mod files for dependency caching
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Build daemon binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/daemon ./src/cmd/daemon

# Stage 2: Build frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /build

# Copy frontend files
COPY src/internal/frontend/package*.json ./
RUN npm ci

COPY src/internal/frontend/ ./
RUN npm run build

# Stage 3: Runtime
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata bash

WORKDIR /app

# Copy daemon binary from go-builder
COPY --from=go-builder /app/daemon .

# Copy frontend dist from frontend-builder
COPY --from=frontend-builder /build/dist ./src/internal/frontend/dist

# Copy entrypoint script
COPY scripts/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Create data directory for configs, episodes, logs
RUN mkdir -p /app/data

# Expose port
EXPOSE 8091

# Set environment variables
ENV ENVIRONMENT=prod
ENV PORT=:8091

# Use entrypoint script
ENTRYPOINT ["/docker-entrypoint.sh"]

# Run daemon
CMD ["./daemon"]
