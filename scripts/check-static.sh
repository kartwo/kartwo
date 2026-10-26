#!/bin/sh
# Linux静态产物核对 / Linux Static Artifact Check
# 功能：在开发宿主运行Go校验器，拒绝带动态加载器或共享库依赖的Linux产物
# 作者：仗键天涯(daxing)
# 邮箱：3442535897@qq.com
# 时间：2026-10-26 22:09:19
set -eu
static_root=$(cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$static_root"
static_host_os=$(go env GOHOSTOS)
static_host_arch=$(go env GOHOSTARCH)
GOOS="$static_host_os" GOARCH="$static_host_arch" CGO_ENABLED=0 go run ./scripts/check-static "$@"
