# 开发任务入口 / Developer Task Runner
# 功能：本地一键复现 CI 的构建/测试/生成/安全门禁
# 作者：仗键天涯(daxing) ｜ 邮箱：3442535897@qq.com ｜ 时间：2026-06-17 17:05:46
.PHONY: all build test vet lint vuln gen tidy run check

all: check

gen:           ## 由 sqlc 重新生成数据层代码
	sqlc generate

tidy:          ## 整理依赖
	go mod tidy

vet:
	go vet ./...

test:
	go test -race -count=1 ./...

build:         ## 构建单静态二进制
	CGO_ENABLED=0 go build -o kartwo ./cmd/kartwo

lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

run: build     ## 本地启动（默认 :8080、./data）
	./kartwo

check: vet test build lint vuln  ## 合主干前本地全量门禁
	@echo "✅ 全部门禁通过"
