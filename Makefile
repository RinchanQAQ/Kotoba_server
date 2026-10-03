# kotoba 后端常用命令入口。
#
#   make help          查看全部命令
#   make run           本地启动 API（默认 development 环境）
#   make test          运行单元测试
#   make build         编译 API 与迁移工具
#   make migrate-up    执行数据库迁移
#   make docker-up     一键起 MySQL + Redis + 迁移 + API
#
# 变量均可在命令行覆盖，例如：
#   make run ENV=production CONFIG=configs/config.yaml
#   make migrate-down STEP=3

APP_NAME    := kotoba
BIN_DIR     := bin

GO          ?= go
# Windows 下可执行文件带 .exe 后缀，Linux/macOS 下为空。
EXE         := $(if $(findstring windows,$(shell $(GO) env GOOS)),.exe,)
API_BIN     := $(BIN_DIR)/api$(EXE)
MIGRATE_BIN := $(BIN_DIR)/migrate$(EXE)

ENV         ?= development
CONFIG      ?= configs/config.yaml
STEP        ?= 1
TO_VERSION  ?= 0
COVER_FILE  := coverage.out

# 竞态检测依赖 cgo 与 C 编译器，未启用时自动跳过（可显式指定 RACE=-race 强制开启）。
RACE        ?= $(if $(filter 1,$(shell $(GO) env CGO_ENABLED)),-race,)

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)
MIGRATE_LDFLAGS := -s -w

.DEFAULT_GOAL := help

.PHONY: help run build build-migrate clean fmt vet test test-cover tidy check \
        migrate-up migrate-down migrate-version migrate-force migrate-drop \
        docker-build docker-up docker-down docker-logs

## help: 显示可用命令
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | awk -F': ' '{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

## run: 以 ENV 指定环境启动 API
run:
	$(GO) run ./cmd -config $(CONFIG) -env $(ENV)

## build: 编译 API 与迁移工具到 bin/
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(API_BIN) ./cmd
	$(GO) build -trimpath -ldflags "$(MIGRATE_LDFLAGS)" -o $(MIGRATE_BIN) ./cmd/migrate

## build-migrate: 只编译迁移工具
build-migrate:
	$(GO) build -trimpath -ldflags "$(MIGRATE_LDFLAGS)" -o $(MIGRATE_BIN) ./cmd/migrate

## fmt: gofmt 格式化
fmt:
	$(GO) fmt ./...

## vet: go vet 静态检查
vet:
	$(GO) vet ./...

## test: 运行全部单元测试（RACE=-race 可强制开启竞态检测）
test:
	$(GO) test $(RACE) -count=1 ./...

## test-cover: 生成覆盖率报告并打印总覆盖率
test-cover:
	$(GO) test $(RACE) -count=1 -coverprofile=$(COVER_FILE) ./...
	$(GO) tool cover -func=$(COVER_FILE) | tail -n 1

## tidy: 整理 go.mod / go.sum
tidy:
	$(GO) mod tidy

## check: 提交前完整检查（格式 + 静态检查 + 测试 + 编译）
check: fmt vet test build

## migrate-up: 升级数据库到最新版本
migrate-up:
	$(GO) run ./cmd/migrate -config $(CONFIG) -env $(ENV) up

## migrate-down: 回退 STEP 个版本（默认 1）
migrate-down:
	$(GO) run ./cmd/migrate -config $(CONFIG) -env $(ENV) -steps $(STEP) down

## migrate-version: 查看当前迁移版本与 dirty 状态
migrate-version:
	$(GO) run ./cmd/migrate -config $(CONFIG) -env $(ENV) version

## migrate-force: 修复 dirty 状态，用法 make migrate-force TO_VERSION=2
migrate-force:
	$(GO) run ./cmd/migrate -config $(CONFIG) -env $(ENV) force $(TO_VERSION)

## migrate-drop: 删除全部表（危险，仅限本地）
migrate-drop:
	$(GO) run ./cmd/migrate -config $(CONFIG) -env $(ENV) drop

## docker-build: 构建 API 镜像
docker-build:
	docker build -t $(APP_NAME)-api:$(VERSION) .

## docker-up: 启动 MySQL + Redis + 迁移 + API
docker-up:
	docker compose up -d --build

## docker-down: 停止容器（保留数据卷）
docker-down:
	docker compose down

## docker-logs: 跟踪 API 与迁移日志
docker-logs:
	docker compose logs -f api migrate

## clean: 清理编译产物与覆盖率文件
clean:
	rm -rf $(BIN_DIR) $(COVER_FILE) coverage.html
