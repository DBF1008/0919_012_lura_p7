#!/usr/bin/env bash
# test.sh - 运行全部单元测试与集成测试（与 Makefile 的 test 目标一致）
#
# 用法:
#   ./test.sh            # 构建插件并运行所有测试
#   SKIP_PLUGINS=1 ./test.sh   # 跳过 .so 插件构建（已构建过时）
set -uo pipefail
cd "$(dirname "$0")"

FAILED=0
run() {
    echo ""
    echo "==> $*"
    "$@" || FAILED=1
}

# 1. 代码生成与插件构建（同 Makefile 的 generate 目标）
if [ "${SKIP_PLUGINS:-0}" != "1" ]; then
    run go generate ./...
    run go build -buildmode=plugin -o ./transport/http/client/plugin/tests/lura-client-example.so ./transport/http/client/plugin/tests
    run go build -buildmode=plugin -o ./transport/http/server/plugin/tests/lura-server-example.so ./transport/http/server/plugin/tests
    run go build -buildmode=plugin -o ./proxy/plugin/tests/lura-request-modifier-example.so ./proxy/plugin/tests/logger
    run go build -buildmode=plugin -o ./proxy/plugin/tests/lura-error-example.so ./proxy/plugin/tests/error
fi

# 2. 单元测试（race + 覆盖率）
run go test -cover -race ./...

# 3. 集成测试
run go test -tags integration ./test/...
run go test -tags integration ./transport/...
run go test -tags integration ./proxy/...

echo ""
if [ "$FAILED" -ne 0 ]; then
    echo "RESULT: FAILED"
    exit 1
fi
echo "RESULT: OK"
