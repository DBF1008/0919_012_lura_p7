#!/usr/bin/env bash
# Runs the full unit/integration test suite, including the plugin .so fixtures.
# Usage: ./test.sh
set -euo pipefail

cd "$(dirname "$0")"

echo "==> go generate ./..."
go generate ./...

echo "==> Building plugin fixtures (.so)"
go build -buildmode=plugin -o ./transport/http/client/plugin/tests/lura-client-example.so ./transport/http/client/plugin/tests
go build -buildmode=plugin -o ./transport/http/server/plugin/tests/lura-server-example.so ./transport/http/server/plugin/tests
go build -buildmode=plugin -o ./proxy/plugin/tests/lura-request-modifier-example.so ./proxy/plugin/tests/logger
go build -buildmode=plugin -o ./proxy/plugin/tests/lura-error-example.so ./proxy/plugin/tests/error

echo "==> Unit tests (race detector)"
go test -cover -race ./...

echo "==> Integration tests: ./test/..."
go test -tags integration ./test/...

echo "==> Integration tests: ./transport/..."
go test -tags integration ./transport/...

echo "==> Integration tests: ./proxy/..."
go test -tags integration ./proxy/...

echo "==> All tests passed"
