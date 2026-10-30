package service

import (
	"context"
	"strconv"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// ImportPartners 按行导入主体，先校验、再按 dryRun 决定是否落库。
//
// 为什么分两步：一份几百行的文件里错两三行是常态。直接导进去之后
// "哪几行没进来"没人说得清；先出一份报告，改完再导，才是可解释的流程。
//
// 整份文件**不做事务**：一行失败不该让另外 299 行白导。已导入的行在报告里
// 有计数，失败行列出原因，用户可以只把那几行改好再导一次——
// 重复导同一行会被编码唯一索引挡住，所以重导是安全的。
func (s *partnerService) ImportPartners(
	ctx context.Context, rows []dto.PartnerImportRow, dryRun bool,
) (*dto.PartnerImportReport, error) {
	if len(rows) == 0 {
		return nil, errs.BadRequest("文件里没有数据行")
	}
	if len(rows) > maxImportRows {
		return nil, errs.BadRequest("单次最多导入 1000 行，请拆成多个文件")
	}

	// 已有主体一次取全：逐行去查库，几百行就是几百次全表扫描。
	existing, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	codeTaken := make(map[string]struct{}, len(existing))
	for _, p := range existing {
		codeTaken[strings.ToUpper(strings.TrimSpace(p.Code))] = struct{}{}
	}

	report := &dto.PartnerImportReport{
		DryRun:     dryRun,
		Total:      len(rows),
		Failures:   make([]dto.PartnerImportFailure, 0),
		Duplicates: make([]dto.PartnerImportDuplicate, 0),
	}

	for _, row := range rows {
		code := strings.TrimSpace(row.Code)
		name := strings.TrimSpace(row.Name)

		fail := func(reason string) {
			report.Failures = append(report.Failures, dto.PartnerImportFailure{
				Line: row.Line, Code: code, Name: name, Reason: reason,
			})
		}

		if code == "" {
			fail("编码为空")
			continue
		}
		if name == "" {
			fail("名称为空")
			continue
		}
		upper := strings.ToUpper(code)
		if _, ok := codeTaken[upper]; ok {
			// 库内已有，或同一份文件里前面出现过——两种情况都是"这行导不进去"。
			fail("编码已存在：" + code)
			continue
		}

		role, ok := parseImportRole(row.Role)
		if !ok {
			fail("角色无法识别（可填 customer / supplier / forwarder，或 客户 / 供应商 / 货代）")
			continue
		}

		// 查重只提示不阻断：与新建页的规则是同一份实现。
		candidates := model.FindPartnerDuplicates(existing, model.PartnerProbe{
			Name: name, Country: row.Country, Email: row.Email, Phone: row.Phone,
		})
		if len(candidates) > 0 {
			reasons := make([]string, 0, len(candidates))
			for _, c := range candidates {
				reasons = append(reasons, c.Reasons...)
			}
			report.Duplicates = append(report.Duplicates, dto.PartnerImportDuplicate{
				Line: row.Line, Code: code, Name: name, Reasons: dedupeStrings(reasons),
			})
		}

		codeTaken[upper] = struct{}{}

		if dryRun {
			report.Skipped++
			continue
		}

		creditDays, err := parseImportInt(row.CreditDays)
		if err != nil {
			fail("账期必须是整数天")
			continue
		}
		creditLimit := money.Zero()
		if raw := strings.TrimSpace(row.CreditLimit); raw != "" {
			parsed, err := money.Parse(raw)
			if err != nil {
				fail("信用额度不是合法数字")
				continue
			}
			creditLimit = parsed
		}

		m := &model.Partner{
			Code: code, Name: name, Role: role,
			EntityType: model.PartnerEntityCompany,
			Country:    row.Country, ContactName: row.ContactName,
			Phone: row.Phone, Email: row.Email, Address: row.Address,
			CreditDays:  creditDays,
			CreditLimit: creditLimit,
		}
		if actor, ok := ActorFrom(ctx); ok {
			m.OwnerID = actor.UserID
		}
		if err := s.repo.Create(ctx, m); err != nil {
			fail("写入失败：" + err.Error())
			continue
		}
		// 新建的行也要参与后面行的查重比对。
		existing = append(existing, *m)
		report.Created++
	}

	s.log.Info("导入主体", "total", report.Total, "created", report.Created,
		"failed", len(report.Failures), "dry_run", dryRun)
	return report, nil
}

// maxImportRows 是单次导入的行数上限：再多的文件该拆，或者该走备份还原。
const maxImportRows = 1000

// parseImportRole 解析角色列。
//
// 同时接受导出文件里的 `customer+supplier` 与中文写法：导出→改→导入
// 是最自然的用法，模板里的值必须原样能吃回去。
func parseImportRole(raw string) (model.PartnerRole, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}

	var role model.PartnerRole
	for _, token := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '+' || r == '|' || r == '/' || r == '，' || r == ','
	}) {
		bit, ok := importRoleBits[strings.ToLower(strings.TrimSpace(token))]
		if !ok {
			return 0, false
		}
		role |= bit
	}
	return role, role != 0
}

var importRoleBits = map[string]model.PartnerRole{
	"customer":  model.PartnerRoleCustomer,
	"客户":        model.PartnerRoleCustomer,
	"supplier":  model.PartnerRoleSupplier,
	"供应商":       model.PartnerRoleSupplier,
	"forwarder": model.PartnerRoleForwarder,
	"货代":        model.PartnerRoleForwarder,
}

func parseImportInt(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
