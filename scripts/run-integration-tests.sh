#!/bin/bash

set -e

echo "Starting integration test environment..."

# Build and start all services, including the test service
# The test service will run the tests and exit, causing docker compose to stop
echo "Building and starting containers..."
docker compose -f docker-compose.test.yml up --build --abort-on-container-exit

# Capture exit code from docker compose
test_exit_code=$?

# Stop containers (in case they're still running)
echo "Stopping containers..."
docker compose -f docker-compose.test.yml down -v

# Show logs if tests failed
if [ $test_exit_code -ne 0 ]; then
    echo "Integration tests failed! Showing logs..."
    docker compose -f docker-compose.test.yml logs test
    docker compose -f docker-compose.test.yml logs daemon
    exit $test_exit_code
fi

echo "Integration tests passed!"
exit 0
