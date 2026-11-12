#!/bin/sh
# 容器入口：把「以什么身份运行」和「运行什么」两件事收在一处。
#
#   1) PUID=0（默认）：不降权，直接以 root exec 主程序。
#   2) PUID≠0：用 groupmod / usermod 把 app 的 uid/gid 对齐到 PUID/PGID，
#      把 /app 属主一并改过来，再 su-exec 落到 PUID:PGID。
#   3) 参数装配：以 - 开头的参数原样转给 server（docker run <image> -version
#      应当打印版本，而不是把它当配置文件路径）；命令是 server 且没带 -config 时
#      补上默认配置（镜像的 CMD 就是 server，所以默认启动读的是 config.prod.yaml）。
#
# 刻意不做的事：
#   - 不解析、不改写配置：库地址、CORS、日志格式都在挂进来的配置文件里；
#     密码与密钥走环境变量（GOWORK_DB_PASSWORD / GOWORK_REDIS_PASSWORD / GOWORK_JWT_SECRET）。
#   - 不额外封装数据库迁移：结构是否同步由配置里的 database.auto_migrate 决定，
#     服务启动时自己按它执行，入口脚本不提供第二个开关。
set -eu

APP_BIN="/app/server"
APP_USER="app"
# 镜像自己的工作目录：二进制、configs、以及可能的只读挂载都在这里。
APP_HOME="/app"
# 默认配置路径；部署侧想换文件可以挂到别处并设置 CONFIG_FILE。
CONFIG_FILE="${CONFIG_FILE:-/app/configs/config.prod.yaml}"

PUID="${PUID:-0}"
PGID="${PGID:-0}"

# 非数字会在 groupmod / usermod 里变成一串难懂的报错，这里提前拦住。
case "${PUID}:${PGID}" in
    *[!0-9:]*)
        echo "[entrypoint][ERROR] PUID/PGID 必须是数字，当前为 PUID=${PUID} PGID=${PGID}" >&2
        exit 1
        ;;
esac

# 参数装配三条路径：没给命令 -> 用 server；首参数以 - 开头 -> 那是 server 的开关，
# 前面补二进制；其余 -> 显式命令，原样执行。
if [ "$#" -eq 0 ]; then
    set -- "${APP_BIN}"
fi
case "$1" in
    -*) set -- "${APP_BIN}" "$@" ;;
esac

# 命令就是 server 且没自己带 -config 时补默认配置。
# 镜像的 CMD 给的是二进制本身（而不是 "/app/server -config …"）：默认配置路径只写在
# 这里一处，否则 CMD 与脚本各存一份，换配置文件时必然漏掉一边。
if [ "$1" = "${APP_BIN}" ]; then
    has_config=0
    for arg in "$@"; do
        case "${arg}" in
            -config|--config|-config=*|--config=*) has_config=1 ;;
        esac
    done
    if [ "${has_config}" -eq 0 ]; then
        set -- "$@" -config "${CONFIG_FILE}"
    fi
fi

if [ "$(id -u)" != "0" ]; then
    # 以 --user 启动时改不了 uid/gid，也改不了属主，只能按现有身份跑。
    echo "[entrypoint][WARN] 当前非 root（uid $(id -u)），跳过 uid/gid 切换与属主对齐" >&2
    exec "$@"
fi

# PUID=0 是镜像默认值：不降权，整条降权链路都不走。
#   - 不改 ID：把 app 改成 uid 0 只会在 /etc/passwd 里多出一个重复的 uid=0；
#   - 不对齐属主：root 本来就能读写任意属主的文件，chown 只会把宿主目录里的文件
#     改成 root 属主，挂载内容多时还会拖慢启动；
#   - 不经 su-exec：0:0 的 su-exec 与直接 exec 等价，少一层进程包装。
if [ "${PUID}" = "0" ]; then
    [ "${PGID}" = "0" ] || echo "[entrypoint][WARN] PUID=0 以 root 运行，PGID=${PGID} 不生效" >&2
    exec "$@"
fi

# 直接改 app 的 ID，不新建用户：容器内 /etc/passwd 与实际身份始终一致。
# -o 不可省：PUID/PGID 可能与已有 ID 相同，不带 -o 会因 ID 冲突失败。
# 改不动（例如 /etc 被只读挂载）也不该让容器起不来：身份与属主都按数字走，
# 用户名只影响 ps / id 的显示，所以告警后继续。
if ! groupmod -o -g "${PGID}" "${APP_USER}" >/dev/null 2>&1; then
    echo "[entrypoint][WARN] groupmod 失败，app 的 gid 保持原值（${PGID} 仍用于属主与运行身份）" >&2
fi
if ! usermod -o -u "${PUID}" "${APP_USER}" >/dev/null 2>&1; then
    echo "[entrypoint][WARN] usermod 失败，app 的 uid 保持原值（${PUID} 仍用于属主与运行身份）" >&2
fi

# 只对齐 /app：二进制、configs、以及部署侧挂上来的目录都在这里。
# 失败静默忽略：configs 常以 :ro 挂载，对只读挂载 chown 必然 EROFS，这是正常现象；
# 真写不进去时服务自己会报 EACCES，比在这里中断更容易定位。
chown -R "${PUID}:${PGID}" "${APP_HOME}" 2>/dev/null || true

echo "[entrypoint] 以 ${PUID}:${PGID} 运行 $(basename "$1")"
exec su-exec "${PUID}:${PGID}" "$@"
