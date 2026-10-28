# 后端镜像（多阶段构建）。
# 构建上下文是 apps/backend 目录：
#   docker build -t gowork-server ./apps/backend
# 基础镜像版本跟随 go.mod 的 go 指令，否则构建时还要临时下载新版工具链。
FROM golang:1.26-alpine AS builder

WORKDIR /src

# 先拷依赖清单，利用层缓存：改代码不会重新下载模块。
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/server /app/server
COPY configs /app/configs

EXPOSE 8080

ENTRYPOINT ["/app/server", "-config", "configs/config.prod.yaml"]
