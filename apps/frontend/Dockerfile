# 前端镜像：nginx 托管构建好的 dist，并把 /api 反代给后端容器。
# 构建上下文是 apps/frontend（不是仓库根目录）：
#   docker build -t gowork-frontend apps/frontend
# 与后端镜像是两个镜像，不合并：两端只靠 HTTP 耦合，各自可独立回滚（见根 AGENTS.md 第 2 节）。
# 前端代码一律用 /api 相对路径，换后端地址改的是 nginx 的上游，不用重编前端。

# ---------- 构建阶段 ----------
# 构建阶段用 glibc 的 bookworm-slim 而不是 alpine：vite / rollup / esbuild 都带原生二进制，
# 在 musl 上要另取一套变体，本地与 CI 踩过一次就是「装得上、跑不起来」。
# 最终镜像里只有 dist 这些静态文件，运行阶段仍是 alpine，这一步不影响产物。
FROM node:22-bookworm-slim AS builder

# pnpm 版本取自 package.json 的 packageManager：本地、CI、镜像三处各写一个版本号，
# 装出来的依赖树就可能不同。corepack 只负责按那个字段把对应版本取下来。
RUN corepack enable

WORKDIR /src

# 先只拷依赖清单：改业务代码不会触发重新装依赖。
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
# 依赖下载目录挂成 BuildKit 缓存：同一台机器上重跑构建不必再从 registry 拉一遍。
# 后端 Dockerfile 用了 $BUILDPLATFORM，本文件也依赖 BuildKit，这条 mount 不会额外抬高门槛。
RUN --mount=type=cache,id=pnpm-store,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

COPY . .
# build 就是 tsc --noEmit && vite build：类型检查与出产物绑在一起，
# 免得「CI 检查过的那份」与「发布出去的这份」是两次不同的构建。
RUN pnpm run build

# ---------- 运行阶段 ----------
FROM nginx:1.28-alpine

# 版本与来源信息由发布流水线传入。metadata-action 也会打同名标签，
# 这里再写一遍是为了让本地 --build-arg 构建出来的镜像同样说得清自己是谁。
ARG VERSION=
ARG COMMIT=none
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

# nginx 的 access log 时间戳读的是 /etc/localtime：不装 tzdata 会与后端日志差 8 小时，
# 排查「前端报 502 的那一刻后端在干什么」时对不上号。
ENV TZ=Asia/Shanghai
RUN apk add --no-cache tzdata \
 && ln -snf "/usr/share/zoneinfo/${TZ}" /etc/localtime \
 && echo "${TZ}" > /etc/timezone

# 后端地址：部署侧用这个环境变量覆盖（compose 里给的是后端容器名 http://gowork-server:8080），
# 默认值只保证镜像单独跑时有个人样的上游。模板在容器启动时由官方 nginx 镜像的 entrypoint
# 做 envsubst，见 nginx/default.conf.template。
ENV GOWORK_API_UPSTREAM=http://server:8080

COPY --from=builder /src/dist /usr/share/nginx/html
COPY nginx/default.conf.template /etc/nginx/templates/default.conf.template

EXPOSE 80

# 探针打这个容器自己的 /healthz（见模板），不穿到后端：
# 后端没起来时该报的是「/api 502」，不该连前端容器一起判死。
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1/healthz || exit 1
