package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/csvx"
	"github.com/edwinhuish/gowork/internal/pkg/response"
)

// exportPageSize 是翻页取数时的每页条数：与列表接口的上限一致（200），
// 用它才能一页一页把结果翻完而不撞上限。
const exportPageSize = 200

// exportMaxRows 是单次导出的行数上限。
//
// 有上限而不是"随便导"：导出接口绕过了列表的分页，几万行的响应会把
// 内存和浏览器一起吃掉；真需要全量时应该走备份，而不是点一个按钮。
const exportMaxRows = 10000

// fetchAll 按页把查询结果翻完。
//
// 复用列表服务而不是另写一份导出 SQL：导出的每一行都必须与界面上看到的**同一批**——
// 另写一份查询，数据范围（T-32）与筛选条件迟早会与列表不一致，
// 而拿到文件的人不会去想"为什么导出里多了一家客户"。
func fetchAll[T any](fetch func(page, size int) ([]T, int64, error)) ([]T, error) {
	out := make([]T, 0, exportPageSize)
	for page := 1; len(out) < exportMaxRows; page++ {
		rows, total, err := fetch(page, exportPageSize)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if len(rows) < exportPageSize || int64(len(out)) >= total {
			break
		}
	}
	if len(out) > exportMaxRows {
		out = out[:exportMaxRows]
	}
	return out, nil
}

// writeCSV 统一处理响应头与写出。
//
// 文件名带日期：同一个目录里下过两次，靠日期才分得清哪份是新的。
func writeCSV(c *gin.Context, filename string, header []string, rows [][]string) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s-%s.csv"`, filename, time.Now().Format("20060102")))
	c.Status(http.StatusOK)
	if err := csvx.Write(c.Writer, header, rows); err != nil {
		// 头已经发出去了，这里只能记日志：再写一个 JSON 错误体只会污染文件。
		c.Error(err) //nolint:errcheck // gin 的 Error 只是把错误收进 c.Errors
	}
}

// ExportPartners GET /api/v1/exports/partners
//
// 参数与客户 / 供应商 / 货代列表完全一致（role / keyword / status / country …），
// 导出的就是列表上那一批：数据范围与筛选条件都复用列表服务。
func (h *PartnerHandler) Export(c *gin.Context) {
	var q dto.PartnerQuery
	if !bindQuery(c, &q) {
		return
	}

	partners, err := fetchAll(func(page, size int) ([]model.Partner, int64, error) {
		q.Page, q.PageSize = page, size
		res, err := h.partners.List(c.Request.Context(), q)
		if err != nil {
			return nil, 0, err
		}
		return res.List, res.Total, nil
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	rows := make([][]string, 0, len(partners))
	for _, p := range partners {
		rows = append(rows, []string{
			p.Code, p.Name, p.Role.String(), p.Country,
			p.ContactName, p.Phone, p.Email, p.Address,
			p.OwnerName, fmt.Sprintf("%d", p.CreditDays), p.CreditLimit.String(),
		})
	}

	writeCSV(c, "partners",
		[]string{"编码", "名称", "角色", "国家/地区", "联系人", "电话", "邮箱", "地址",
			"负责人", "账期(天)", "信用额度"},
		rows)
}

// ExportProducts GET /api/v1/exports/products
//
// 成本价按 T-36 的规矩：业务员导出的成本价是空的，不是 0——
// 导出的文件会被转发出去，0 在那里同样会被读成"这件货不要钱"。
func (h *ProductHandler) Export(c *gin.Context) {
	var q dto.ProductQuery
	if !bindQuery(c, &q) {
		return
	}

	products, err := fetchAll(func(page, size int) ([]model.Product, int64, error) {
		q.Page, q.PageSize = page, size
		res, err := h.products.List(c.Request.Context(), q)
		if err != nil {
			return nil, 0, err
		}
		return res.List, res.Total, nil
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	rows := make([][]string, 0, len(products))
	for _, p := range products {
		cost := ""
		if !p.CostHidden {
			cost = p.CostPrice.String()
		}
		rows = append(rows, []string{
			p.SKUCode, p.NameCN, p.NameEN, p.Category, p.Spec, p.Unit,
			cost, p.SalePrice.String(),
			p.HSCode, statusText(p.Status),
		})
	}

	writeCSV(c, "products",
		[]string{"SKU", "品名", "英文品名", "品类", "规格", "单位",
			"成本价", "售价", "HS编码", "状态"},
		rows)
}

// statusText 把启用状态写成人话：文件是给人看的，"1" 需要另一个文档来解释。
func statusText(status model.EnableStatus) string {
	if status == model.EnableStatusOn {
		return "启用"
	}
	return "停用"
}
