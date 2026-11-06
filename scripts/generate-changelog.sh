#!/usr/bin/env bash
# 从提交历史生成 Release 的说明（body），交给 gh release create/edit 的
# --notes-file 写入。
#
# 用法：
#   generate-changelog.sh <当前标签> [上一个标签] [输出文件]
#
#   <当前标签>    必填，如 v0.2.0
#   [上一个标签]  由调用方用 get-tags.sh 算好再传进来；留空则退化成
#                 「当前标签的全部历史」
#   [输出文件]    默认 release-notes.md
#
# 环境变量：
#   GITHUB_REPOSITORY     有则用它拼对比链接，否则从 git remote 推断
#   GITHUB_STEP_SUMMARY   设置了就一并写进 Actions 的步骤摘要
#   GITHUB_OUTPUT         设置了就输出 notes_file（供工作流后续步骤引用）
#
# 为什么不用 GitHub 自己生成的那份（gh release create --generate-notes）：
#   它按提交作者分批，且依赖提交邮箱是否与 GitHub 账号绑定——绑不上 / 绑错人时，
#   说明里会出现一串陌生名字或一个 @ 都 @ 不到的账户，而这恰恰没人会去复核。
#   自己拼一次 git log 没有任何网络依赖，发布路径上也就少一个会失败、会限流的环节。
#
# 为什么不按「约定式提交」筛掉不带前缀的提交：本仓库的提交标题是中文短句
# （如「出货列表看不出这票是给哪个客户的」），前缀（feat/fix/docs）时有时无。
# 只认前缀那套过滤器会把没前缀的全部丢弃，于是每个版本的说明都变成「本版本
# 没有可归类的改动」。因此改成：认得出前缀的按类型分节，认不出的统一进
# 「其他改动」——一条不丢。

set -euo pipefail

if [ $# -lt 1 ] || [ $# -gt 3 ]; then
  echo "用法: $0 <当前标签> [上一个标签] [输出文件]" >&2
  exit 1
fi

current_tag="$1"
previous_tag="${2:-}"
output_file="${3:-release-notes.md}"

# 拼对比链接用的仓库地址。取不到只会让链接缺一段，不该让发布失败。
repo_slug="${GITHUB_REPOSITORY:-}"
if [ -z "$repo_slug" ]; then
  # 两步剥壳（先前缀、再 .git 后缀）而不是一条带可选组的正则：
  #   `[^/]+?`（惰性）在 POSIX ERE 里不是标准语法，GNU sed 会把它当贪婪处理，
  #   于是 `…/repo.git` 会整个吃进结果里、`.git` 留在末尾。
  repo_slug=$(git config --get remote.origin.url 2>/dev/null \
    | sed -E 's#^.*github\.com[:/]##; s#\.git$##' || true)
  # ★ sed 在**不匹配**时会把原串原样还回来（例如 remote 是本地路径），所以必须
  #   再验一次形状：否则拼出来的是一个「看起来像链接」的错误 URL，而不是一句警告。
  if ! [[ "$repo_slug" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]]; then
    repo_slug=""
  fi
fi
if [ -z "$repo_slug" ]; then
  echo "::warning::推断不出 GitHub 仓库地址，发布说明里只会给出文字范围、不带对比链接" >&2
fi

# 当前标签必须存在：不然下面的 git log 会以「未知修订」失败，报错指不到根因。
if ! git rev-parse -q --verify "refs/tags/$current_tag" >/dev/null; then
  echo "::error::标签 $current_tag 在本地不存在。浅克隆或未拉标签时先 git fetch --tags。" >&2
  exit 1
fi

# 显式传入的上一个标签也必须真实存在：标签被重命名、或本地是浅克隆时，
# git log 会直接失败把整个发布卡住 —— 而这些情形都只需退回「全量历史」就能继续。
if [ -n "$previous_tag" ] && ! git rev-parse -q --verify "refs/tags/$previous_tag" >/dev/null; then
  echo "::warning::标签 $previous_tag 在本地不存在，退回「${current_tag} 的全部历史」" >&2
  previous_tag=""
fi

if [ -z "$previous_tag" ]; then
  commit_range="$current_tag"
  echo "没有可用的上一个版本标签：${current_tag} 的全部历史都会进说明"
else
  commit_range="${previous_tag}..${current_tag}"
  echo "对比范围：${commit_range}"
fi

# 分隔符用 \x1f（单元分隔符）而不是制表符：提交标题里出现制表符虽然罕见，但一旦
# 出现就会被 IFS 切成两列，作者名落到标题里。
#
# --no-merges：合并提交本身不含改动，放进说明只会多一行噪音（真正的改动在它
# 引用的那些提交里）。
commits=$(git log --no-merges --pretty=format:"%s%x1f%an%x1f%h" "$commit_range" || true)

# 类型必须在行首：写成 `*feat*` 时 "fix the feature list" 会同时出现在
# 「新功能」与「缺陷修复」两节里。
commit_type_re='^(feat|fix|perf|refactor|docs|chore|ci|test|build|style)(\([^)]*\))?!?:'

sections_dir=$(mktemp -d)
trap 'rm -rf "$sections_dir"' EXIT

# 没有归类文件时不创建 —— 下面按 [ -s ] 判断小节是否渲染。
if [ -n "$commits" ]; then
  while IFS=$'\x1f' read -r subject author short_sha; do
    [ -n "$subject" ] || continue
    if [[ "$subject" =~ $commit_type_re ]]; then
      type="${BASH_REMATCH[1]}"
    else
      type="other"
    fi
    # 反引号会吞掉其后整段 Markdown：标题里落单的反引号要转义。
    printf '%s\n' "- ${subject//\`/\\\`} —— **${author}** (\`${short_sha}\`)" \
      >> "$sections_dir/$type"
  done <<< "$commits"
fi

commit_count=$(git rev-list --no-merges --count "$commit_range" 2>/dev/null || echo 0)

notes=""
has_section=false

append_section() {
  local section_file="$1" section_title="$2"
  if [ -s "$section_file" ]; then
    notes="${notes}### ${section_title}"$'\n\n'
    notes="${notes}$(cat "$section_file")"$'\n\n'
    has_section=true
  fi
}

if [ -n "$previous_tag" ]; then
  notes="${notes}本次发布包含 ${commit_count} 个提交，来自 \`${previous_tag}\` 到 \`${current_tag}\`。"$'\n\n'
else
  notes="${notes}本次发布包含 ${commit_count} 个提交（仓库的首次发布，范围是整个提交历史）。"$'\n\n'
fi

append_section "$sections_dir/feat" '新功能'
append_section "$sections_dir/fix" '缺陷修复'
append_section "$sections_dir/perf" '性能优化'
append_section "$sections_dir/refactor" '代码重构'
append_section "$sections_dir/docs" '文档'
append_section "$sections_dir/chore" '日常维护'
append_section "$sections_dir/ci" 'CI 与流水线'
append_section "$sections_dir/test" '测试'
append_section "$sections_dir/build" '构建'
append_section "$sections_dir/style" '代码格式'
append_section "$sections_dir/other" '其他改动'

# 一个小节都没拼出来时也必须给出文字：让 Release 页面只有一堆产物、没有变更内容，
# 正是那种「看起来发布成功了」的静默失败。
if [ "$has_section" = false ]; then
  notes="${notes}该范围内没有可列出的提交（通常是重复打了同一个标签，或范围内全是合并提交）。"$'\n\n'
fi

notes="${notes}"$'\n'"---"$'\n\n'

if [ -n "$previous_tag" ]; then
  range_label="${previous_tag}...${current_tag}"
  range_url="compare/${previous_tag}...${current_tag}"
else
  range_label="${current_tag} 的全部提交"
  range_url="commits/${current_tag}"
fi

# 这行必定包含当前标签，「断言发布说明非空且与本次标签有关」就靠它当判据。
if [ -n "$repo_slug" ]; then
  notes="${notes}**完整变更**：[\`${range_label}\`](https://github.com/${repo_slug}/${range_url})"$'\n'
else
  # 没有仓库地址时宁可只给文字范围，也不要拼出 https://github.com//... 这种无效链接
  notes="${notes}**完整变更**：\`${range_label}\`"$'\n'
fi

printf '%s\n' "$notes" > "$output_file"
echo "发布说明已写入 $output_file："
cat "$output_file"

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  printf '%s\n' "$notes" >> "$GITHUB_STEP_SUMMARY"
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "notes_file=$output_file" >> "$GITHUB_OUTPUT"
fi

exit 0
