# syntax=docker/dockerfile:1

# ---------- 构建阶段 ----------
FROM golang:1.27-alpine AS builder

WORKDIR /src

# 先只拷贝依赖清单，利用层缓存：go.mod/go.sum 没变时不会重复下载依赖。
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# 版本信息通过 ldflags 注入，无需改动源码。
ARG VERSION=dev
ARG BUILD_TIME=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
        -o /out/api ./cmd \
 && CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w" \
        -o /out/migrate ./cmd/migrate

# ---------- 运行阶段 ----------
FROM alpine:3.22 AS runtime

# ca-certificates：调用外部 HTTPS 服务需要；tzdata：与数据库时区保持一致。
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S -g 10001 app \
 && adduser -S -u 10001 -G app app

ENV TZ=Asia/Shanghai

WORKDIR /app

COPY --from=builder /out/api /app/api
COPY --from=builder /out/migrate /app/migrate
# 只拷贝默认配置，密码等敏感项一律通过环境变量注入（见 docker-compose.yml）。
COPY configs/config.yaml configs/config.dev.yaml configs/config.prod.yaml /app/configs/

# 以非 root 用户运行。
USER app

EXPOSE 8080

# 健康检查直接用二进制自带的 -health-check，镜像无需安装 curl/wget。
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/app/api", "-health-check"]

ENTRYPOINT ["/app/api"]
