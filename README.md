# kotoba server

Go 1.27 + Gin 后端基建：多环境配置、结构化日志与链路追踪、统一错误/响应格式、
MySQL + GORM 与版本化迁移、常用中间件、容器化部署、Makefile 入口。

## 目录结构

```
cmd/
  main.go            API 进程：加载配置 → 初始化依赖 → 启动 HTTP → 优雅退出
  migrate/main.go    迁移进程：up / down / version / force / drop
configs/
  config.yaml        基础配置（所有环境共享）
  config.dev.yaml    开发环境覆盖
  config.prod.yaml   生产环境覆盖
  config.local.yaml.example  本机私密覆盖示例（真实文件不入库）
internal/
  app/router.go      gin 引擎组装：中间件顺序、兜底路由、探针、业务路由
  bootstrap/         配置(viper) / 日志(zap) / MySQL(gorm) / Redis 初始化
  middleware/        requestID、请求日志、recover、错误收敛、CORS
  migrations/        内嵌 SQL 迁移脚本与 migrate 实例入口
  modules/<name>/    业务模块，自行提供 RegisterRoutes(v1, app)
  shared/            response / errcode / validator / logging / constant
```

## 快速开始

```bash
cp configs/config.local.yaml.example configs/config.local.yaml   # 填本机数据库密码
make migrate-up                                                  # 建表
make run                                                         # 默认 development
curl http://127.0.0.1:8080/healthz
curl -X POST http://127.0.0.1:8080/v1/echo -H 'Content-Type: application/json' -d '{"message":"hi"}'
```

容器方式（MySQL + Redis + 迁移 + API 全自动）：

```bash
cp .env.example .env
docker compose up -d --build
docker compose logs -f api
docker compose down          # 加 -v 连数据卷一起删
```

## 配置管理

加载顺序，后者覆盖前者：

| 顺序 | 来源 | 说明 |
| --- | --- | --- |
| 1 | `configs/config.yaml` | 基础配置 |
| 2 | `configs/config.<env>.yaml` | 环境覆盖，支持 `prod` / `production` 两种命名 |
| 3 | `configs/config.local.yaml` | 本机私密覆盖，已 gitignore |
| 4 | 环境变量 | `KOTOBA_` 前缀，点号换下划线 |

环境由 `--env` > `KOTOBA_ENV` > `APP_ENV` > 默认 `development` 决定。示例：

```bash
make run ENV=production                              # 加载 config.yaml + config.prod.yaml
KOTOBA_MYSQL_PASSWORD=secret make run                # 环境变量覆盖 mysql.password
KOTOBA_APP_CORS_ALLOW_ORIGINS="https://a.com" make run   # 列表也可用逗号分隔覆盖
```

配置在启动时完成默认值填充与合法性校验，**生产环境必须显式配置 `app.cors.allow_origins`**，
否则直接启动失败，避免误放开任意跨域来源。`bootstrap.Config.Sources` 会记录实际加载了哪些文件，
并在启动日志中输出。

## 日志与链路追踪

- zap 结构化日志，`log.format` 为 `console`（开发，带颜色）或 `json`（生产）。
- 每个请求由 `middleware.RequestID` 生成/透传 `X-Request-ID`（调用方带了就沿用），
  响应头回写同一个 ID。
- `middleware.RequestLogger` 把带 `request_id` 字段的 logger 注入 `context`，
  业务层统一用 `logging.From(ctx)` 取用：

  ```go
  logging.From(c.Request.Context()).Info("下单成功", zap.Int64("order_id", id))
  ```

- GORM 的 SQL 日志同样接入 zap（`bootstrap/gormlog.go`）：只要使用
  `db.WithContext(c.Request.Context())`，SQL 日志就会带上同一个 `request_id`，
  HTTP 日志与 SQL 日志可以用一个 ID 串起来；慢查询（默认 >200ms）以 warn 级别单独记录。

## 统一错误与响应格式

所有业务接口返回同一结构（探针除外）：

```json
{ "code": 0, "message": "ok", "data": {...}, "request_id": "..." }
```

- 业务错误：`errcode.Error`（`Code` 面向调用方，`Status` 决定 HTTP 状态码，`Err` 只进日志）。
- handler 写响应：`response.OK / Created / NoContent / Fail`；参数校验用
  `response.BindJSON / BindQuery / BindURI`，失败时响应已写好，直接 `return`。
- handler 也可以只 `c.Error(err)`，由 `middleware.ErrorHandler` 统一转成响应体。
- 未知错误统一归一为 500「服务器内部错误」，底层原因不会泄漏到响应体。
- 校验提示为中文且使用 json 字段名：`{"message":""}` → `message 为必填项`。
- 404 / 405 兜底路由同样返回统一格式，不会出现 gin 默认的纯文本。

错误码分段：`1xxxx` 通用、`2xxxx` 鉴权、`3xxxx` 词库、`4xxxx` 学习、`5xxxx` 服务端。

## 数据库与迁移

- GORM + MySQL，连接池参数来自 `mysql.*` 配置，启动时做一次连通性检查。
- 迁移用 golang-migrate 的版本化 SQL 文件，脚本通过 `go:embed` 打进二进制，
  容器/部署环境不需要额外挂载 SQL 目录；版本状态记录在 `schema_migrations` 表。
- 新增迁移：在 `internal/migrations/` 下按 `<6 位版本号>_<描述>.{up,down}.sql` 成对添加，
  版本号必须连续（有测试守着命名、配对与可解析性）。

```bash
make migrate-up                       # 升级到最新
make migrate-down STEP=2              # 回退 2 个版本
make migrate-version                  # 当前版本与 dirty 状态
make migrate-force TO_VERSION=2       # 修复 dirty：人工确认库结构后对齐版本号
make migrate-drop                     # 删表（危险，仅本地）
```

## 中间件

`internal/app/router.go` 中的注册顺序即执行顺序：

| 顺序 | 中间件 | 作用 |
| --- | --- | --- |
| 1 | `RequestID` | 生成/透传链路 ID，回写响应头 |
| 2 | `RequestLogger` | 注入请求级 logger（带 `request_id`） |
| 3 | `AccessLog` | 访问日志，按状态码分级（5xx error / 4xx warn / 其他 info），并记录原始错误 |
| 4 | `Recovery` | 捕获 panic，记录堆栈，返回统一 500 |
| 5 | `ErrorHandler` | 收敛 `c.Error` 上报的错误 |
| 6 | `CORS` | 白名单跨域；非白名单预检直接 403 |

## Makefile

```bash
make help          # 列出全部目标
make run           # go run ./cmd -config ... -env ...
make test          # 单元测试（cgo 可用时自动加 -race）
make test-cover    # 覆盖率报告
make build         # 编译 API 与迁移工具到 bin/
make check         # fmt + vet + test + build
make migrate-up    # 数据库迁移
make docker-up     # 一键起全套服务
```

变量可覆盖：`make run ENV=production`、`make migrate-down STEP=3`。

## 探针与容器

- `GET /healthz`：存活探针，只反映进程状态。
- `GET /readyz`：就绪探针，检查 MySQL 与 Redis 连通性。
- 镜像多阶段构建（`golang:1.27-alpine` → `alpine:3.22`），非 root 运行；
  健康检查使用二进制自带的 `api -health-check`，镜像内无需 curl/wget。
- `docker-compose.yml` 中 `api` 依赖 `mysql`/`redis` 健康 + `migrate` 成功退出，
  保证启动顺序正确。
