# 开发任务入口 / Developer Task Runner
# 功能：本地一键复现 CI 的构建/测试/生成/安全门禁
# 作者：仗键天涯(daxing) ｜ 邮箱：3442535897@qq.com ｜ 时间：2026-06-17 17:05:46
#
# 工具查找顺序：优先项目内 ./.bin（go install 到此，不进 git），再回退系统 PATH。
# 注：用每条 recipe 内显式注入 PATH，兼容 macOS 自带的老 GNU make 3.81。
BIN := $(CURDIR)/.bin
TOOLPATH := PATH="$(BIN):$$PATH"

.PHONY: all build test vet lint vuln gen tidy run check tools

all: check

tools:         ## 安装钉死版本的开发工具到 ./.bin（版本与 CI 一致）
	GOBIN=$(BIN) go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
	GOBIN=$(BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0
	GOBIN=$(BIN) go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
	@echo "✅ 工具已装入 ./.bin（gitleaks 请用 brew install gitleaks）"

gen:           ## 由 sqlc 重新生成数据层代码
	$(TOOLPATH) sqlc generate

tidy:          ## 整理依赖
	go mod tidy

vet:
	go vet ./...

test:
	go test -race -count=1 ./...

build:         ## 构建单静态二进制
	CGO_ENABLED=0 go build -o kartwo ./cmd/kartwo

lint:
	$(TOOLPATH) golangci-lint run ./...

vuln:
	$(TOOLPATH) govulncheck ./...

run: build     ## 本地启动（默认 :8080、./data）
	./kartwo

check: vet test build lint vuln  ## 合主干前本地全量门禁
	@echo "✅ 全部门禁通过"
