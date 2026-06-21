#!/usr/bin/env bash
set -euo pipefail

# 默认构建输出路径为 /tmp/credential-priority.so
# 允许通过 CPA_BUILD_OUTPUT_PATH 环境变量覆盖输出路径
OUTPUT_PATH="${CPA_BUILD_OUTPUT_PATH:-/tmp/credential-priority.so}"

echo "Building credential-priority plugin..."
echo "Target output: ${OUTPUT_PATH}"

# 执行 Go 动态链接库构建
go build -buildmode=c-shared -o "${OUTPUT_PATH}" .

echo "Build succeeded."
