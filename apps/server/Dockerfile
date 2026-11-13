# 后端镜像。构建上下文就是本目录（apps/server）：
#   docker build -t gowork-server -f apps/server/Dockerfile apps/server
#   task server:image        # 同上，版本号缺省取 `git describe --always --long --dirty`
#
# 镜像不从仓库里读任何文件取版本号：只认 --build-arg VERSION，不传就是 dev。
# 仓库根目录的 VERSION.txt 只服务 `task server:build`（本地编译二进制），
# 镜像这条路由调用方把值传进来——镜像因此完全不依赖仓库根，上下文也只有本目录。
#
# 为什么不在这里跑 git describe：--dirty 判的是索引与工作区文件的差异，而构建上下文
# 只有本目录（根目录的 md、docs/、.git 都不在里面），在容器里算出来会永远带 -dirty，
# 比不给版本号更误导。要真版本号就在宿主上算好传进来。

# ---------- 构建阶段 ----------
# 编译固定在构建机原生架构上：多架构构建时 arm64 不跑在 QEMU 里。
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

# GOTOOLCHAIN=local：只用镜像里的工具链，go.mod 抬高 go 指令时直接失败，
# 不静默下载一个新版本 —— 否则同一份代码在两台机器上编出的结果可能不同。
ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local

WORKDIR /src

# 先只拷依赖清单：改业务代码不会触发重新下载模块。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 构建参数声明在这里而不是文件开头：ARG 的值会进入其后每一步的环境变量，
# 摆在 go mod download 之前的话，换一个版本号（BUILD_DATE 每次都不同）就连
# 模块下载那一步的缓存一起打掉 —— 实测每次多花 160s 重下一遍依赖。
#
# TARGETOS / TARGETARCH 由 buildx 传入；本地普通构建回退到当前平台。
# VERSION / COMMIT / BUILD_DATE 由调用方传入：发布流水线给 tag 名，本地
# `task server:image` 给 `git describe --always --long --dirty`，都不传时落到 dev。
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN set -eu; \
    if [ -z "${VERSION}" ]; then \
      echo "未传 --build-arg VERSION：版本号记为 dev（要真版本号用 task server:image）" >&2; \
    fi; \
    ver="${VERSION:-dev}"; \
    arch="${TARGETARCH:-$(go env GOARCH)}"; \
    echo "building version=${ver} os=${TARGETOS} arch=${arch}"; \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${arch}" go build -trimpath \
      -ldflags "-s -w -X main.version=${ver} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
      -o /out/server ./cmd/server

# ---------- 运行阶段 ----------
FROM alpine:3.20

# PUID / PGID 是运行时约定：0 表示以 root 运行，非 0 时由 entrypoint 降权到该身份。
# TZ 决定 Go 的 time.Local：单据日期与汇率取当天都按它算。
ENV TZ=Asia/Shanghai \
    PUID=0 \
    PGID=0

# ca-certificates: 连外部数据库 / 缓存时校验 TLS 证书
# tzdata: TZ 生效的前提
# su-exec: entrypoint 按 PUID/PGID 切换身份
# shadow: 提供 groupmod / usermod，entrypoint 用它把 app 的 uid/gid 对齐到 PUID/PGID
# app 只按 1000:1000 建出来当占位，镜像刻意不写 USER：改 uid/gid 与对齐属主都需要 root，
# 降权统一在入口脚本里做。
RUN apk add --no-cache ca-certificates tzdata su-exec shadow \
 && addgroup -g 1000 app \
 && adduser -D -H -u 1000 -G app -h /app -s /sbin/nologin app \
 && mkdir -p /app \
 && chown -R app:app /app

# alpine 的 tzdata 不写 /etc/localtime：Go 只认 TZ，但容器里的 date 读的是它，
# 不补会出现「日志是北京时间、date 是 UTC」两套时间。
RUN ln -snf "/usr/share/zoneinfo/${TZ}" /etc/localtime \
 && echo "${TZ}" > /etc/timezone

WORKDIR /app

COPY --from=builder /out/server /app/server
# 配置样例一起进镜像：部署侧要么挂载覆盖 config.prod.yaml，要么直接用 -config 指别处。
COPY configs /app/configs
COPY entrypoint.sh /entrypoint.sh

# Windows 检出（core.autocrlf）会带 CRLF，先归一化再给执行权限。
RUN sed -i 's/\r$//' /entrypoint.sh \
 && chmod 0755 /entrypoint.sh /app/server

EXPOSE 8080

# 存活探针：/healthz 不碰数据库也不鉴权，进程能响应就算活着。
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1

# ENTRYPOINT 做降权与参数装配，CMD 只回答「默认跑什么」：
#   docker run <image>                 -> server -config /app/configs/config.prod.yaml
#   docker run <image> -version        -> server -version（打印版本后退出）
#   docker run <image> -config /x.yaml -> server -config /x.yaml
# CMD 给的是二进制而不是完整命令行：默认配置路径只有入口脚本一处，
# 换配置改 CONFIG_FILE 或直接覆盖 CMD，不必两处对齐。
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/app/server"]
