#!/usr/bin/env bash
# 本地发版：问版本号 -> 同步前后端版本号 -> 提交、打 tag -> 问是否推送。
#
# 用法（经 Taskfile 调，见根目录 `task --list-all`）：
#   task release                        # 第一个问题是版本号（回车 = 当前版本 +1 patch），
#                                       # 本地做完再问是否推送（回车 = 推）
#   task release VERSION=1.2.0          # 指定版本号（带不带 v 前缀都认），不再问版本号
#   task release VERSION=1.3.0-rc.1     # 预发布也走同一条路，tag 仍是 v 前缀
#   task release REMOTE=upstream        # 推送到别的远程（默认 origin；没配远程就跳过推送）
#   task release PUSH=false             # 只做本地部分（提交 + tag），不问也不推
#   task release DRY_RUN=true           # 只打印将发生什么，不改文件、不提交、不推
#
# 也可以直接跑脚本，参数与上面等价：
#   scripts/release.sh --version 1.2.0 --remote origin --no-push
#
# 为什么把这一串放在脚本里，而不是在 Taskfile 里连写几条 git：
#   发布是「改文件 → 提交 → 打 tag → 推」的链式动作，每一环都可能失败，而且失败时
#   用户要立刻知道「停在哪一步、已经落地的部分怎么补」。前置校验（工作区是否干净、
#   远程是否可达、tag 是否已被占用）在脚本里能写清楚原因，在 Taskfile 里只能堆
#   sh: 条件，报错就只剩一句 git 的原始输出。
#
# 版本号的两个落点是刻意的，不是重复：
#   仓库根目录 VERSION.txt 是项目的正本：`task server:build` 缺省读它，构建期用
#   -ldflags "-X main.version=…" 注进二进制，供 `gowork-server -version` 自证；
#   前端 apps/web/package.json 的 version 是 npm 生态认的那个。
#   两处不一致时，报障的人报的数字就对不上产物 —— 所以这里总是一起改。

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  # 直接把文件头注释当帮助输出（到第一个空行为止），省得用法在这里再抄一遍、然后跟实现走散。
  sed -n '2,/^$/p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

# ---- 参数 ----
# 环境变量与命令行参数并存：Taskfile 走环境变量（env:），直接调脚本走参数。
requested_version="${VERSION:-}"
remote="${REMOTE:-origin}"
remote_given=0
push_mode="${PUSH:-ask}"
dry_run="${DRY_RUN:-false}"

need_value() {
  if [ "$#" -lt 2 ] || [ -z "${2:-}" ]; then
    echo "错误：$1 需要一个值。" >&2
    exit 1
  fi
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    -v | --version)
      need_value "$@"
      requested_version="$2"
      shift 2
      ;;
    --version=*)
      requested_version="${1#*=}"
      shift
      ;;
    -r | --remote)
      need_value "$@"
      remote="$2"
      remote_given=1
      shift 2
      ;;
    --remote=*)
      remote="${1#*=}"
      remote_given=1
      shift
      ;;
    --no-push)
      push_mode="no"
      shift
      ;;
    --push)
      push_mode="yes"
      shift
      ;;
    -n | --dry-run)
      dry_run="true"
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "错误：未知参数 $1" >&2
      echo >&2
      usage >&2
      exit 1
      ;;
  esac
done

# 布尔类变量统一在这里归一：别让「以为只是预览 / 以为不会推」的写法悄悄真的执行。
case "${dry_run,,}" in
  1 | true | yes | y) dry_run="true" ;;
  *) dry_run="false" ;;
esac

# PUSH 三态：ask（默认，推之前问一句）/ yes（不问直接推）/ no（不问、也不推）。
# no 是给「只想在本地做个 tag 看看」留的口子 —— 远程没通时不必为了一条 tag 去修网络。
case "${push_mode,,}" in
  "" | ask) push_mode="ask" ;;
  1 | true | yes | y) push_mode="yes" ;;
  0 | false | no | n) push_mode="no" ;;
  *)
    echo "错误：PUSH 只认 ask / true / false，拿到的是 '$push_mode'。" >&2
    exit 1
    ;;
esac

# ---- 定位仓库 ----
# 一律先跳到仓库根：下面的路径都是相对根的，在 apps/server 里跑也不该出错。
if ! repo_root="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  echo "错误：当前目录不在 Git 仓库里。发布要靠 git 打 tag 并推送，先进入仓库再执行。" >&2
  echo "      脚本位置：$script_dir" >&2
  exit 1
fi
cd "$repo_root"

version_file="VERSION.txt"
web_pkg="apps/web/package.json"

for f in "$version_file" "$web_pkg"; do
  if [ ! -f "$f" ]; then
    echo "错误：找不到 $f。发布要同步的版本文件不在预期位置，先确认仓库结构没变。" >&2
    exit 1
  fi
done

# ---- 前置校验 1：工作区必须干净 ----
# 发布会在版本号文件上写东西并提交。混着别人的改动提交，事后没人分得清 tag 里哪一部分
# 是发布本身带来的，也没法安全回滚。
if [ -n "$(git status --porcelain)" ]; then
  echo "错误：工作区有未提交的变更，发布前请先提交或 stash。" >&2
  echo "      版本号文件需要被一并提交，混进别的改动会让这个 tag 说不清包含什么：" >&2
  echo >&2
  git status --short >&2
  exit 1
fi

# ---- 前置校验 2：得在一个分支上 ----
# detached HEAD 上打出来的 tag 推得出去，但它不在任何分支上，别人 checkout 分支时拿不到
# 这次发布的代码 —— 等到有人发现「tag 里的提交不在主干上」就已经晚了。
if ! branch="$(git symbolic-ref --quiet --short HEAD)"; then
  echo "错误：当前处于 detached HEAD，无法确定要推送哪个分支。" >&2
  echo "      先切回分支（例如 git switch master）再发布。" >&2
  exit 1
fi

# ---- 前置校验 3：远程（没配就跳过推送，不算失败） ----
# 默认的 origin 不存在时允许只做本地发布：本地提交与 tag 本身有价值，没必要因为
# 还没连通远程就把整条流程堵死。但 REMOTE= 点名的远程不存在就是另一回事了 ——
# 那几乎总是打错了名字，静默跳过会让人以为推出去了。
push_target=""
if git remote get-url "$remote" >/dev/null 2>&1; then
  push_target="$remote"
elif [ "$remote_given" = "1" ]; then
  echo "错误：没有名为 $remote 的远程仓库。当前已有：" >&2
  if [ -z "$(git remote)" ]; then
    echo "      （一个都没有：先 git remote add origin <url>）" >&2
  else
    git remote -v >&2
  fi
  exit 1
else
  echo "提示：没有配置名为 $remote 的远程仓库，本次只做本地发布，推送这步会跳过。"
fi

# ---- 版本号：读现有、算默认值 ----
# 只认语义化版本：主版本.次版本.修订号，可带 -预发布 与 +构建元数据。
# 不用 git describe 之类的宽松判断：tag 名是给 release.yml 的 semver 解析和
# docker/metadata-action 用的，格式一旦跑偏，坏的是发布流水线而不是这条命令。
is_semver() {
  [[ "$1" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]
}

# 最近的稳定 tag（vX.Y.Z，不含 -rc / -beta 这类预发布），按语义化版本排序取最大。
# 用 version:refname 排序而不是字典序：字典序会把 v0.10.0 排到 v0.9.0 前面。
latest_stable_tag() {
  git tag --sort=version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | tail -1 || true
}

read_pkg_version() {
  sed -nE 's/^[[:space:]]*"version":[[:space:]]*"([^"]+)".*/\1/p' "$web_pkg" | head -1
}

# 当前版本 = 最新稳定 tag、根目录 VERSION.txt、前端 package.json 三者里最大的那个。
# 正常发布后三者相等（每次一起更新）；取最大值是为了在有人手改过其中一处时，
# 默认值不会倒退到已经发过的号上 —— 那种情况 git tag 会直接拒绝，白问一遍。
current_version() {
  {
    printf '%s\n' "$(latest_stable_tag)" | sed 's/^v//'
    tr -d '[:space:]' < "$version_file" 2>/dev/null || true
    printf '\n'
    read_pkg_version
  } | grep -E '^[0-9]+\.[0-9]+\.[0-9]+' | sort -V | tail -1 || true
}

# 只 bump 数字主干：1.0.0-rc.1 的下一个 patch 记作 1.0.1（去掉后缀再 +1）。
# 不玩「预发布转正成 1.0.0」那种推断 —— 那是在猜用户意图，问一句比猜准。
bump_patch() {
  local core="${1%%-*}" major minor patch
  IFS=. read -r major minor patch <<< "$core"
  if [ -z "${patch:-}" ] || ! [[ "$patch" =~ ^[0-9]+$ ]]; then
    return 1
  fi
  printf '%s.%s.%s' "$major" "$minor" "$((patch + 1))"
}

default_version=""
base_version="$(current_version)"
if [ -n "$base_version" ] && is_semver "$base_version"; then
  default_version="$(bump_patch "$base_version")" || default_version=""
fi
if [ -z "$default_version" ]; then
  echo "错误：推不出默认版本号（最新 tag 与 $web_pkg 的 version 都是空的或不可解析）。" >&2
  echo "      用 VERSION=1.2.3 明确指定。" >&2
  exit 1
fi

# ---- 交互 1：版本号 ----
# 提示一律写 stderr，stdout 只留「结果」—— 这样下面用 $(ask_version) 取值时抓得到干净的值。
# stdin 不是终端（被脚本调用、输入被重定向）时不问：挂在 read 上等一个永远不会来的回车，
# 比直接用默认值糟糕得多。
ask_version() {
  local suggested="$1" input
  if [ ! -t 0 ]; then
    echo "标准输入不是终端，版本号取默认值 $suggested（要指定就加 VERSION=…）" >&2
    printf '%s' "$suggested"
    return 0
  fi
  while true; do
    printf '当前[%s]（默认 %s）：' "$base_version" "$suggested" >&2
    if ! IFS= read -r input; then
      # Ctrl-D / 输入被关掉：当成回车处理。死循环或带着空版本号往下走都更难解释。
      echo >&2
      input=""
    fi
    input="${input#v}"
    input="${input//[[:space:]]/}"
    if [ -z "$input" ]; then
      printf '%s' "$suggested"
      return 0
    fi
    if is_semver "$input"; then
      printf '%s' "$input"
      return 0
    fi
    echo "  '$input' 不是语义化版本（形如 1.2.3 / 1.2.3-rc.1），重来。" >&2
  done
}

if [ -n "$requested_version" ]; then
  version="${requested_version#v}"
  if ! is_semver "$version"; then
    echo "错误：版本号 '$requested_version' 不符合语义化版本（形如 1.2.3 / 1.2.3-rc.1）。" >&2
    exit 1
  fi
  reason="由 VERSION 指定"
else
  version="$(ask_version "$default_version")"
  if [ "$version" = "$default_version" ]; then
    reason="默认值：当前 $base_version +1 个 patch"
  else
    reason="手动输入（默认值曾是 $default_version）"
  fi
fi

tag="v$version"

# tag 已存在就别往下走：git tag 会失败，而这时版本号文件已经改过、可能已经提交，
# 留下一个「改了版本号却没有对应 tag」的提交，比一开始就拒绝要难收拾得多。
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  echo "错误：tag $tag 在本地已存在。换一个版本号（VERSION=...），或先删掉旧 tag。" >&2
  exit 1
fi

# 这次到底推不推：提前算好，dry-run 与结尾的收尾信息都用它，免得两处各猜一遍。
if [ -z "$push_target" ]; then
  push_plan="跳过推送（没有配远程 $remote）"
elif [ "$push_mode" = "no" ]; then
  push_plan="跳过推送（PUSH=false）"
elif [ "$push_mode" = "yes" ]; then
  push_plan="推送到 $push_target（PUSH=true，不再询问）"
else
  push_plan="本地做完后再问是否推送到 $push_target"
fi

# ---- 前置校验 4：远程可达、且远程上没有同名 tag ----
# 这条要连网，所以放在动手改文件之前：等到推送才发现远程不可达或号被占了，
# 本地已经多出一个提交和一个 tag，得手工回退。
# PUSH=false 时整段跳过 —— 本地发布不该被网络状况挡住。
if [ -n "$push_target" ] && [ "$push_mode" != "no" ]; then
  if ! remote_refs="$(git ls-remote --tags "$push_target" 2>/dev/null)"; then
    echo "错误：读不到远程 $push_target（网络不通，或没有该仓库的权限）。" >&2
    echo "      修好远程访问再试，或加 PUSH=false 先只做本地发布：git ls-remote --tags $push_target" >&2
    exit 1
  fi
  # 按字段精确比对而不是 grep 一个带点的正则：tag 里的 "." 在正则里是任意字符，
  # 用 grep 会把 v1x2x3 也算成同一个 tag。
  if printf '%s\n' "$remote_refs" \
    | awk -v ref="refs/tags/$tag" '$2 == ref || $2 == ref "^{}" { hit = 1 } END { exit !hit }'; then
    echo "错误：远程 $push_target 上已经有 tag $tag。换一个版本号（VERSION=...）。" >&2
    exit 1
  fi
fi

# ---- 只预览 ----
if [ "$dry_run" = "true" ]; then
  cat <<EOF
[dry-run] 版本号：$version（$reason）
[dry-run] tag：$tag
[dry-run] 分支：$branch
[dry-run] 将写入：$version_file（根目录，后端构建读它）
[dry-run]          $web_pkg 的 version
[dry-run] 之后会：提交 -> 打 tag -> $push_plan
（dry-run 没有改动任何文件，也没有提交或推送）
EOF
  exit 0
fi

if ! command -v node >/dev/null 2>&1; then
  echo "错误：找不到 node。前端版本号按 package.json 的结构改写，没有 node 就只能靠正则猜格式，" >&2
  echo "      而 package.json 内部也可能出现 \"version\" 键。装好 node 再试（前端构建本来也需要它）。" >&2
  exit 1
fi

# ---- 同步前后端版本号 ----
# 前端走 node 解析/回写 JSON：python 不一定有，sed 换一行在嵌套结构里会改错地方。
node -e '
const fs = require("fs");
const [file, next] = process.argv.slice(1);
const pkg = JSON.parse(fs.readFileSync(file, "utf8"));
pkg.version = next;
// 两空格缩进 + 结尾换行，与仓库现有格式一致：否则 diff 里会多出一条「整个文件都变了」，
// 真正的版本号变化就被埋掉了。
fs.writeFileSync(file, JSON.stringify(pkg, null, 2) + "\n");
' "$web_pkg" "$version"

# 根目录 VERSION.txt 只有一行数字：`task server:build` 会原样读它去拼 -ldflags，
# 多一个空格、多一行注释都会被当成版本号的一部分注进二进制。
printf '%s\n' "$version" > "$version_file"

# ---- 提交 ----
# 版本号本来就和目标一致时（比如上次发布推失败、这次原样重来），不要造一个空提交：
# 空提交会让 release 说明里多一条没有内容的条目。
git add "$version_file" "$web_pkg"
if git diff --cached --quiet; then
  echo "版本号已经是 $version，跳过版本提交，直接打 tag"
else
  git commit -q -m "发布版本： $tag"
fi

# ---- 打 tag ----
# 带注释的 tag：git show 里能看到发布说明与打 tag 的时间，而轻量 tag 只是一个指针，
# 事后翻历史时看不出这一次发布是「什么时候、为了什么」打的。
git tag -a "$tag" -m "发布 $tag"

# ---- 交互 2：推送 ----
# 本地部分到这里已经全部完成（提交 + tag 都在），推不推只影响「别人看不看得见」。
# 所以这一步不推也算成功，只是把补推命令留在屏幕上。
should_push=0
skip_reason=""
if [ -z "$push_target" ]; then
  skip_reason="没有配置远程仓库（$remote）"
elif [ "$push_mode" = "no" ]; then
  skip_reason="按 PUSH=false 跳过推送"
elif [ "$push_mode" = "yes" ]; then
  should_push=1
else
  reply=""
  if [ ! -t 0 ]; then
    # 非交互：按「回车」处理，也就是推。不想推就显式写 PUSH=false。
    echo "标准输入不是终端，按默认继续推送（不想推就加 PUSH=false）"
    should_push=1
  else
    printf '推送到 %s ？[Y/n] ' "$push_target" >&2
    if ! IFS= read -r reply; then
      echo >&2
      reply=""
    fi
    case "${reply}" in
      "" | [yY] | [yY][eE][sS]) should_push=1 ;;
      *) skip_reason="已选择不推送" ;;
    esac
  fi
fi

if [ "$should_push" = "1" ]; then
  # --atomic：分支与 tag 要么一起成功、要么一起失败。分开推的话，分支推上去而 tag 失败
  # 会留下一个「代码已发布、却没有触发 release.yml」的中间态，得人工补推。
  if ! git push --atomic "$push_target" "HEAD:refs/heads/$branch" "refs/tags/$tag"; then
    cat >&2 <<EOF

错误：推送失败。本地已完成的部分都还在，没有被丢掉：
    提交：$(git rev-parse --short HEAD)
    tag ：$tag
修好远程访问（网络 / 权限 / 分支是否被保护）后补推即可：
    git push $push_target HEAD:refs/heads/$branch
    git push $push_target refs/tags/$tag
远程不支持 --atomic 时也会走到这里：按上面两条分开推，git 的报错会指明卡在哪一条。
EOF
    exit 1
  fi

  exit 0
fi

# 没推：把本地状态和补推命令摆在屏幕上，别让人以为发布失败了。
echo
echo "$skip_reason。本地已经完成的都在这里："
echo "    提交：$(git rev-parse --short HEAD)"
echo "    tag ：$tag（版本号 $version，前后端已同步）"
if [ -z "$(git remote)" ]; then
  echo "    补推前先加远程：git remote add origin <url>"
fi
cat <<EOF
之后要推的话：
    git push $remote HEAD:refs/heads/$branch
    git push $remote refs/tags/$tag
（推 tag 才会触发 .github/workflows/release.yml；这两条就是同一次发布剩下的部分。）
EOF
