# syntax=docker/dockerfile:1
#
# Hexagon 多阶段构建 Dockerfile
#
# 构建目标:
#   - app (默认): 主应用服务
#   - devui: Dev UI（Go 后端提供 API + 静态文件服务）
#
# 使用方式:
#   # 构建主应用
#   docker build -t hexagon -f deploy/Dockerfile .
#
#   # 构建 DevUI 服务
#   docker build -t hexagon-devui --target devui -f deploy/Dockerfile .
#
#   # 带版本号构建
#   docker build --build-arg VERSION=v0.4.0 -t hexagon:v0.4.0 -f deploy/Dockerfile .
#
#   # 多架构构建
#   docker buildx build --platform linux/amd64,linux/arm64 -t hexagon -f deploy/Dockerfile .

# ============== 构建参数 ==============
ARG GO_VERSION=1.23
ARG ALPINE_VERSION=3.20

# ============== Go 构建阶段 ==============
FROM golang:${GO_VERSION}-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# 依赖层缓存：go.mod/go.sum 变化才重新下载
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 源码层
COPY . .

# 构建二进制（静态链接，支持多架构）
ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /bin/hexagon \
    ./cmd/hexagon/

# ============== 运行时基础镜像 ==============
FROM alpine:${ALPINE_VERSION} AS base

RUN apk add --no-cache ca-certificates tzdata \
    && rm -rf /var/cache/apk/*

# 非 root 用户
RUN addgroup -g 10001 -S hexagon \
    && adduser -u 10001 -S hexagon -G hexagon

WORKDIR /app

COPY --from=builder /bin/hexagon /app/hexagon
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

RUN mkdir -p /app/data /app/config \
    && chown -R hexagon:hexagon /app

USER hexagon

# ============== 主应用目标 ==============
FROM base AS app

EXPOSE 8000

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=15s \
    CMD wget --spider -q http://localhost:8000/health || exit 1

ENTRYPOINT ["/app/hexagon"]
CMD ["serve"]

# ============== DevUI 目标 ==============
FROM base AS devui

EXPOSE 8080

ENV DEVUI_ADDR=:8080

HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD wget --spider -q http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/hexagon"]
CMD ["devui"]
