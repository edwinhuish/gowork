#!/usr/bin/env bash
# 查找当前版本标签的「上一个版本标签」，供发布说明确定 git log 的对比范围。
#
# 用法：
#   get-tags.sh [TARGET_TAG]
#
#   TARGET_TAG   可选，默认取 $GITHUB_REF_NAME；本地手动跑时必须显式给出。
#
# 输出（写进 $GITHUB_OUTPUT，没这个环境变量就只打到标准输出）：
#   current_tag   当前版本标签
#   previous_tag  上一个版本标签；没有则为空，此时发布说明等于「当前标签的全部历史」
#
# 为什么不直接在 workflow 里写 `git describe --tags --abbrev=0 当前标签^`：
#   它只认 git 的访达顺序，不认语义化版本 —— 标签按字典序排时 v0.10.0 会落在
#   v0.9.0 前面，跨 minor / major 时算出来的「上一个」常常不是想要的那一个。
#   这里改成显式比版本大小，结果可预测。
#
# 为什么不取「上一个 major / minor 系列的最新标签」：那种做法会把上一个系列
# 的全部提交也算进来，而发布说明应当是增量的 —— v0.2.0 不该重复列 v0.1.9
# 已经发布过的改动。

set -euo pipefail

TARGET_TAG="${1:-${GITHUB_REF_NAME:-}}"

if [ -z "$TARGET_TAG" ]; then
  echo "用法: $0 <当前版本标签（如 v0.2.0）>" >&2
  exit 1
fi

# 标签必须在本地能解析，否则下游 `git log <范围>` 会以「未知修订」失败，
# 而报错信息指不到根因。这也是 CI 里必须 fetch-depth: 0 的原因。
if ! git rev-parse -q --verify "refs/tags/$TARGET_TAG" >/dev/null; then
  echo "::error::标签 $TARGET_TAG 在本地不存在。浅克隆或未拉标签时先 git fetch --tags。" >&2
  exit 1
fi

# 候选标签只认已发布的稳定版本（vX.Y.Z 及其简写），预发布标签（-beta/-rc/-
# alpha/-dev）不当作「上一个版本」—— v0.2.0 的说明不该只列 v0.2.0-rc.1 之后
# 那点改动，那些改动本来就没发布过。
#
# --sort=version:refname 让列表按语义化版本升序（字典序会把 v0.10.0 排到
# v0.9.0 前面，必须避开）。
#
# `|| true` 不可省：set -e 下 grep 一条都匹配不到会让整条赋值以 1 退出，脚本
# 在此静默终止，后面的降级逻辑根本跑不到。
all_tags=$(git tag --sort=version:refname 2>/dev/null \
  | grep -E '^v[0-9]+(\.[0-9]+)*$' \
  | grep -vE -- '-(beta|rc|alpha|dev)' || true)

# 取版本号主干（去掉前缀 v 与预发布后缀），形如 X.Y.Z。
version_key() {
  local v="${1#v}"
  echo "${v%%-*}"
}

# 版本大小比较：$1 < $2 时返回 0。
#
# 分两步做，而不是交给 sort -V 一次判完：GNU sort -V 不认语义化版本的预发布序
# —— 实测它认为 v0.11.0 < v0.11.0-rc.1，于是给 v0.11.0-rc.1 挑出来的「上一个版
# 本」会是还没发布的 v0.11.0 自己。按 semver，带预发布后缀的版本必定小于它的
# 主干版本。
version_lt() {
  local a="$1" b="$2"
  local a_key b_key
  a_key="$(version_key "$a")"
  b_key="$(version_key "$b")"

  # 1) 主干不同：直接比数字。此时两端都是纯数字点分串，sort -V 在这些输入上可靠。
  if [ "$a_key" != "$b_key" ]; then
    [ "$(printf '%s\n%s\n' "$a_key" "$b_key" | sort -V | head -1)" = "$a_key" ]
    return
  fi

  # 2) 主干相同：带预发布后缀的那个更小（v0.11.0-rc.1 < v0.11.0）。
  #    候选列表里已滤掉预发布标签，所以带后缀的只可能是当前标签。
  case "$a" in
    *-*) return 0 ;;
    *) return 1 ;;
  esac
}

previous_tag=""
while IFS= read -r tag; do
  [ -n "$tag" ] || continue
  # 列表是升序的，一旦遇到不小于当前的标签就可以停了 —— 后面的都更大。
  # 顺带让「当前标签是预发布」的情形自然成立：它不在候选列表里（被过滤掉了），
  # 不会被命中，循环停在第一个稳定版为止。
  if version_lt "$tag" "$TARGET_TAG"; then
    previous_tag="$tag"
  else
    break
  fi
done <<< "$all_tags"

if [ -z "$previous_tag" ]; then
  # 首个版本走这条路。不是错误：发布说明会退化成「当前标签的全部历史」。
  echo "没有找到比 $TARGET_TAG 更早的稳定版本标签，发布说明将覆盖该标签的全部历史"
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "current_tag=$TARGET_TAG" >> "$GITHUB_OUTPUT"
  echo "previous_tag=$previous_tag" >> "$GITHUB_OUTPUT"
fi

echo "当前版本标签: $TARGET_TAG"
echo "上一个版本标签: ${previous_tag:-<无>}"

exit 0
