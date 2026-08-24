#!/bin/bash

set -e

echo "Starting integration test environment..."
echo "Building and starting containers..."

# --exit-code-from test e o que torna este script capaz de FALHAR. Com --abort-on-container-exit
# sozinho o compose para a stack quando o container de teste sai, mas devolve 0 de qualquer jeito
# — uma suite quebrada era reportada como verde. O `|| test_exit_code=$?` existe porque o `set -e`
# mataria o script aqui, antes de coletar os logs abaixo.
test_exit_code=0
docker compose -f docker/docker-compose.test.yml up --build \
    --abort-on-container-exit --exit-code-from test || test_exit_code=$?

# Os logs tem que ser lidos ANTES do `down -v`, que remove os containers junto com eles.
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
