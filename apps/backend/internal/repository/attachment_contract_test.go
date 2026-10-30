package repository

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// frontendAttachmentTypes 是前端那份 `AttachmentEntity` 联合类型所在的文件。
//
// 后端这份是判据（不在 attachTargets 里的类型一律 400），前端那份只是"少撞一次 400"。
// 两边靠人肉对齐，而人会漏：补功能时只改了一边，界面上能选、点上传被拒——这条
// 测试就是把它变成一次红灯，而不是等用户来报。
const frontendAttachmentTypes = "../../../frontend/src/api/attachments.ts"

var quotedTypePattern = regexp.MustCompile(`'([a-z_]+)'`)

// TestAttachmentTypesMatchFrontend 比对前后端两份"附件能挂在哪"的清单。
//
// 文件找不到时跳过而不是失败：仓库可能只 checkout 了后端，那种情形下报红是噪音。
func TestAttachmentTypesMatchFrontend(t *testing.T) {
	raw, err := os.ReadFile(frontendAttachmentTypes)
	if err != nil {
		t.Skipf("读不到前端附件类型定义（只 checkout 了后端？）: %v", err)
	}

	block := unionBlock(string(raw), "export type AttachmentEntity")
	if block == "" {
		t.Fatalf("%s 里找不到 AttachmentEntity 的联合类型", frontendAttachmentTypes)
	}

	front := map[string]bool{}
	for _, m := range quotedTypePattern.FindAllStringSubmatch(block, -1) {
		front[m[1]] = true
	}
	if len(front) == 0 {
		t.Fatalf("没能从联合类型里解析出任何类型：\n%s", block)
	}

	var missingInBackend, missingInFrontend []string
	for typ := range front {
		if _, ok := attachTargets[typ]; !ok {
			missingInBackend = append(missingInBackend, typ)
		}
	}
	for typ := range attachTargets {
		if !front[typ] {
			missingInFrontend = append(missingInFrontend, typ)
		}
	}
	sort.Strings(missingInBackend)
	sort.Strings(missingInFrontend)

	if len(missingInBackend) > 0 {
		t.Errorf("前端能选、后端会拒（attachTargets 里没有）：%v", missingInBackend)
	}
	if len(missingInFrontend) > 0 {
		t.Errorf("后端允许、前端没有入口（AttachmentEntity 里没有）：%v", missingInFrontend)
	}
}

// unionBlock 截出 `export type X = ...` 这一段，到空行为止。
// 前端那份联合类型每项占一行、段末是空行，这个切法够用；切不出来时返回空串由调用方报错。
func unionBlock(src, decl string) string {
	i := strings.Index(src, decl)
	if i < 0 {
		return ""
	}
	rest := src[i:]
	if j := strings.Index(rest, "\n\n"); j >= 0 {
		return rest[:j]
	}
	return rest
}
