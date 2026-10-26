#!/bin/sh
# 受限资源验收入口 / Constrained Capacity Runner
# 功能：构建实际Linux程序及驱动，在禁外网1CPU/1GiB容器校验并保留报告，不挂载商家数据
# 作者：仗键天涯(daxing)
# 邮箱：3442535897@qq.com
# 时间：2026-10-26 19:10:49
set -eu
umask 077

case "$0" in
  /*) capacity_script="$0" ;;
  *) capacity_script="$(pwd -P)/$0" ;;
esac
capacity_root=$(cd -- "$(dirname -- "$capacity_script")/.." && pwd -P)
cd "$capacity_root"
capacity_arch=$(docker info --format '{{.Architecture}}')
case "$capacity_arch" in
  aarch64|arm64) capacity_arch=arm64 ;;
  x86_64|amd64) capacity_arch=amd64 ;;
  *) printf 'Unsupported Docker architecture: %s\n' "$capacity_arch" >&2; exit 1 ;;
esac
# Do not fetch a new image implicitly or mount any existing store directory.
docker image inspect ubuntu:24.04 >/dev/null
mkdir -p "$capacity_root/.bin"
capacity_run=$(mktemp -d "$capacity_root/.bin/capacity-profile.XXXXXX")
capacity_commit=$(git rev-parse HEAD)
capacity_version="capacity-$(git describe --always --dirty)"
capacity_uid=$(id -u)
capacity_gid=$(id -g)
printf 'Capacity artifacts: %s\n' "$capacity_run"
docker image inspect ubuntu:24.04 --format '{{.Id}} {{json .RepoDigests}}' > "$capacity_run/image.txt"
printf '%s\n' "$capacity_commit" > "$capacity_run/source-commit.txt"

CGO_ENABLED=0 GOOS=linux GOARCH="$capacity_arch" go build -trimpath \
  -ldflags "-s -w -X main.Version=$capacity_version" -o "$capacity_run/kartwo" ./cmd/kartwo
CGO_ENABLED=0 GOOS=linux GOARCH="$capacity_arch" go test -c \
  -o "$capacity_run/capacity.test" ./internal/server
mkdir "$capacity_run/results"
if docker run --rm --pull=never --network none --cpus 1 --memory 1g --memory-swap 1g \
  --pids-limit 128 --cap-drop ALL --security-opt no-new-privileges --read-only \
  --user "$capacity_uid:$capacity_gid" --tmpfs /tmp:rw,nosuid,nodev,size=64m \
  --mount "type=bind,src=$capacity_run/kartwo,dst=/kartwo,readonly" \
  --mount "type=bind,src=$capacity_run/capacity.test,dst=/capacity.test,readonly" \
  --mount "type=bind,src=$capacity_run/results,dst=/results" \
  --env TMPDIR=/results --env KARTWO_CAPACITY_BINARY=/kartwo \
  --env KARTWO_CAPACITY_OUTPUT=/results --env "KARTWO_CAPACITY_COMMIT=$capacity_commit" \
  ubuntu:24.04 /capacity.test -test.run '^TestConstrainedBinaryCapacityProfile$' \
  -test.count=1 -test.v -test.timeout=5m > "$capacity_run/profile.log" 2>&1; then
  cat "$capacity_run/results/report.json"
  printf '\nVerified capacity report: %s/results/report.json\n' "$capacity_run"
else
  capacity_status=$?
  cat "$capacity_run/profile.log" >&2
  printf 'Capacity profile failed (exit %s); retained artifacts: %s\n' "$capacity_status" "$capacity_run" >&2
  exit "$capacity_status"
fi
