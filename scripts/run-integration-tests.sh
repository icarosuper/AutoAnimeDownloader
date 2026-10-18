#!/bin/bash

set -e

echo "Starting integration test environment..."
echo "Building and starting containers..."

# --exit-code-from test is what makes this script able to FAIL. With --abort-on-container-exit
# alone, compose stops the stack when the test container exits, but still returns 0 —
# a broken suite was reported green. The `|| test_exit_code=$?` exists because `set -e`
# would kill the script here, before collecting the logs below.
test_exit_code=0
docker compose -f docker/docker-compose.test.yml up --build \
    --abort-on-container-exit --exit-code-from test || test_exit_code=$?

# Logs have to be read BEFORE `down -v`, which removes the containers along with them.
if [ $test_exit_code -ne 0 ]; then
    echo "Integration tests failed! Showing logs..."
    docker compose -f docker/docker-compose.test.yml logs test
    docker compose -f docker/docker-compose.test.yml logs daemon
fi

echo "Stopping containers..."
docker compose -f docker/docker-compose.test.yml down -v

if [ $test_exit_code -ne 0 ]; then
    exit $test_exit_code
fi

echo "Integration tests passed!"
