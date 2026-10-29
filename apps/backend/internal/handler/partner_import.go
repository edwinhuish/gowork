package handler

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/response"
)

// ImportPartners POST /partners/imports?dry_run=true|false
//
// 收一份 CSV（multipart，字段名 file）。**导出文件可以直接当模板**：
// 表头与 `/exports/partners` 一致，改完另存 CSV 就能导回来。
//
// dry_run=true 只出报告不落库——前端先让用户看一眼"哪几行会被跳过"，
// 确认之后再以 dry_run=false 真导。
func (h *PartnerHandler) Import(c *gin.Context) {
	dryRun, err := strconv.ParseBool(c.DefaultQuery("dry_run", "true"))
	if err != nil {
		response.Fail(c, errs.BadRequest("dry_run 只能是 true 或 false"))
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		response.Fail(c, errs.BadRequest("请上传 CSV 文件（multipart 字段名 file）"))
		return
	}
	defer func() { _ = file.Close() }()

	rows, err := readPartnerCSV(file)
	if err != nil {
		response.Fail(c, err)
		return
	}

	report, err := h.partners.ImportPartners(c.Request.Context(), rows, dryRun)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, report)
}

// readPartnerCSV 把文件读成按表头映射过的行。
//
// 按表头而不是按列序取值：用户在 Excel 里插一列、调一下顺序是很正常的事，
// 按列序读会在毫无提示的情况下把"电话"填进"邮箱"。
func readPartnerCSV(r io.Reader) ([]dto.PartnerImportRow, error) {
	reader := csv.NewReader(r)
	// 单元格里可能有换行（地址常见），交给 csv 包处理，不限制列数。
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, errs.BadRequest("CSV 解析失败：请确认文件是逗号分隔的纯文本（Excel 另存为 CSV）")
	}
	if len(records) < 2 {
		return nil, errs.BadRequest("文件里没有数据行")
	}

	header := make(map[string]int, len(records[0]))
	for i, cell := range records[0] {
		// 去掉 Excel 存的 BOM 与首尾空白。
		key := strings.TrimSpace(strings.TrimPrefix(cell, "\ufeff"))
		header[key] = i
	}

	// 至少要有编码与名称，否则这份文件不是主体表。
	if _, ok := header["编码"]; !ok {
		return nil, errs.BadRequest("缺少「编码」列：建议先导出一次，用它当模板")
	}
	if _, ok := header["名称"]; !ok {
		return nil, errs.BadRequest("缺少「名称」列：建议先导出一次，用它当模板")
	}

	rows := make([]dto.PartnerImportRow, 0, len(records)-1)
	for i, record := range records[1:] {
		at := func(column string) string {
			idx, ok := header[column]
			if !ok || idx >= len(record) {
				return ""
			}
			return record[idx]
		}
		// 整行全空（Excel 常见的尾部空行）直接跳过，不当成失败行。
		if strings.TrimSpace(strings.Join(record, "")) == "" {
			continue
		}
		rows = append(rows, dto.PartnerImportRow{
			Line:        i + 2, // +1 表头，+1 从 1 起
			Code:        at("编码"),
			Name:        at("名称"),
			Role:        at("角色"),
			Country:     at("国家/地区"),
			ContactName: at("联系人"),
			Phone:       at("电话"),
			Email:       at("邮箱"),
			Address:     at("地址"),
			CreditDays:  at("账期(天)"),
			CreditLimit: at("信用额度"),
		})
	}
	return rows, nil
}
