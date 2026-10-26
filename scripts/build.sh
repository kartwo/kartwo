#!/bin/sh
# 程序构建入口 / Application Build Entry
# 功能：统一发布与开发构建，Linux强制内嵌WASM编解码并禁用CGO
# 作者：仗键天涯(daxing)
# 邮箱：3442535897@qq.com
# 时间：2026-10-26 22:09:19
set -eu
build_root=$(cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$build_root"
build_os=$(go env GOOS)
if [ "$build_os" = linux ]; then
  CGO_ENABLED=0 go build "$@" -tags=nodynamic ./cmd/kartwo
else
  CGO_ENABLED=0 go build "$@" ./cmd/kartwo
fi
