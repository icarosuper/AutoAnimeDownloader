#!/bin/bash

set -e

echo "Starting integration test environment..."

# Start containers
docker compose -f docker-compose.test.yml up -d

# Wait for services to be ready
echo "Waiting for services to be ready..."
sleep 5

# Check if daemon is responding
max_attempts=30
attempt=0
while [ $attempt -lt $max_attempts ]; do
    if curl -s http://localhost:8091/api/v1/status > /dev/null 2>&1; then
        echo "Daemon is ready!"
        break
    fi
    attempt=$((attempt + 1))
    echo "Waiting for daemon... ($attempt/$max_attempts)"
    sleep 2
done

if [ $attempt -eq $max_attempts ]; then
    echo "ERROR: Daemon did not become ready in time"
    docker compose -f docker-compose.test.yml logs daemon
    docker compose -f docker-compose.test.yml down
    exit 1
fi

# Run integration tests
echo "Running integration tests..."
cd src/tests/integration
go test -v ./...

test_exit_code=$?

# Stop containers
echo "Stopping containers..."
cd ../../..
docker compose -f docker-compose.test.yml down

if [ $test_exit_code -eq 0 ]; then
    echo "Integration tests passed!"
    exit 0
else
    echo "Integration tests failed!"
    exit 1
fi
