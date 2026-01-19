# Stage 1: Build frontend (must be first!)
FROM node:20-alpine AS frontend-builder

WORKDIR /build

# Copy frontend files
COPY src/internal/frontend/package*.json ./
RUN npm ci

COPY src/internal/frontend/ ./
RUN npm run build

# Stage 2: Build Go daemon (frontend is embedded)
FROM golang:1.24-alpine AS go-builder

WORKDIR /build

# Copy go mod files for dependency caching
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Copy frontend dist from frontend-builder (needed for embed)
COPY --from=frontend-builder /build/dist ./src/internal/frontend/dist

# Build daemon binary (frontend is embedded via //go:embed)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/daemon ./src/cmd/daemon

# Stage 3: Runtime
FROM alpine:3.19

# Update package index and install dependencies with retry logic
# Retry up to 3 times to handle transient DNS/network issues
RUN for i in 1 2 3; do \
        apk update --no-cache && \
        apk add --no-cache --update-cache ca-certificates tzdata bash wget && \
        break || sleep 5; \
    done

WORKDIR /app

# Copy daemon binary (frontend is already embedded)
COPY --from=go-builder /app/daemon .

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
