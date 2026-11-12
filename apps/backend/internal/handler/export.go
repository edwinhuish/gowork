package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"gowork_server/internal/dto"
	"gowork_server/internal/model"
	"gowork_server/internal/pkg/csvx"
	"gowork_server/internal/pkg/response"
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
// 参数与客户 / 供应商 / 货代列表完全一致（role / keyword / country …），
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
			p.HSCode,
		})
	}

	writeCSV(c, "products",
		[]string{"SKU", "品名", "英文品名", "品类", "规格", "单位",
			"成本价", "售价", "HS编码"},
		rows)
}

// ExportShipments GET /api/v1/exports/shipments
//
// 参数与出货列表一致（status / sales_order_id），导出的就是列表上那一批。
// 一票一行（不是一行一分配行）：出货台账按票看，客户对账也是按票对。
// 箱数与毛净重取整票合计——它们印在装箱单上，也是货代最常来问的三个数。
func (h *ShipmentHandler) ExportShipments(c *gin.Context) {
	var q dto.ShipmentQuery
	if !bindQuery(c, &q) {
		return
	}

	list, err := fetchAll(func(page, size int) ([]model.Shipment, int64, error) {
		q.Page, q.PageSize = page, size
		res, err := h.shipments.List(c.Request.Context(), q)
		if err != nil {
			return nil, 0, err
		}
		return res.List, res.Total, nil
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	out := make([][]string, 0, len(list))
	for _, s := range list {
		cartons, gross, net := s.PackingTotals()
		out = append(out, []string{
			s.DocNo,
			model.DateOnly(s.ShipDate).Format("2006-01-02"),
			s.Vessel, s.BLNo,
			dateOrEmpty(s.ETD), dateOrEmpty(s.ETA),
			strconv.Itoa(cartons),
			gross.StringFixed(3), net.StringFixed(3),
			s.Status.Label(),
		})
	}

	writeCSV(c, "shipments",
		[]string{"单号", "出货日", "船名", "提单号", "ETD", "ETA", "箱数", "毛重(kg)", "净重(kg)", "状态"},
		out)
}

// dateOrEmpty 印日期，nil 留空而不是印一个零值时间。
func dateOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return model.DateOnly(*t).Format("2006-01-02")
}

// ExportSalesOrders GET /api/v1/exports/sales-orders
//
// 参数与订单列表一致（status / partner_id / keyword），导出的就是列表上那一批。
// 原币与本币各一列：业务员看原币，老板看本币，两列都印比事后换算省事。
func (h *SalesOrderHandler) ExportSalesOrders(c *gin.Context) {
	var q dto.SalesOrderQuery
	if !bindQuery(c, &q) {
		return
	}

	list, err := fetchAll(func(page, size int) ([]model.SalesOrder, int64, error) {
		q.Page, q.PageSize = page, size
		res, err := h.orders.List(c.Request.Context(), q)
		if err != nil {
			return nil, 0, err
		}
		return res.List, res.Total, nil
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	out := make([][]string, 0, len(list))
	for _, o := range list {
		partner := ""
		if o.Partner != nil {
			partner = o.Partner.Name
		}
		out = append(out, []string{
			o.DocNo,
			model.DateOnly(o.OrderDate).Format("2006-01-02"),
			partner,
			string(o.TradeTerm),
			o.CurrencyCode,
			o.AmountTotal.String(),
			o.AmountBaseTotal.String(),
			o.Status.Label(),
		})
	}

	writeCSV(c, "sales-orders",
		[]string{"单号", "下单日", "客户", "贸易术语", "币种", "金额(原币)", "金额(本币)", "状态"},
		out)
}

// ExportPayments GET /api/v1/exports/payments?direction=10
//
// 收付款台账：direction=10 是收款、20 是付款，与列表筛的是同一批。
// 汇率一行一列：这张单锁定的是哪个价，对账时这是唯一能解释差额的数字。
func (h *PaymentHandler) ExportPayments(c *gin.Context) {
	var q dto.PaymentQuery
	if !bindQuery(c, &q) {
		return
	}

	list, err := fetchAll(func(page, size int) ([]model.Payment, int64, error) {
		q.Page, q.PageSize = page, size
		res, err := h.payments.List(c.Request.Context(), q)
		if err != nil {
			return nil, 0, err
		}
		return res.List, res.Total, nil
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	out := make([][]string, 0, len(list))
	for _, m := range list {
		partner := ""
		if m.Partner != nil {
			partner = m.Partner.Name
		}
		dir := "收款"
		if m.Direction != model.PaymentDirectionIn {
			dir = "付款"
		}
		out = append(out, []string{
			m.DocNo,
			dir,
			model.DateOnly(m.PayDate).Format("2006-01-02"),
			partner,
			m.CurrencyCode,
			m.Amount.String(),
			m.ExchangeRate.String(),
			m.AmountBase.String(),
			string(m.Method),
		})
	}

	writeCSV(c, "payments",
		[]string{"单号", "方向", "到账日", "对方", "币种", "金额(原币)", "汇率", "金额(本币)", "方式"},
		out)
}

// ExportReceivables GET /api/v1/exports/receivables
//
// 参数与应收列表一致（status / partner_id / overdue / keyword）。
// 未结清 = 金额 − 已核销 − 已开贷项：与核销校验、账龄、客户 360 同一口径，
// 导出里另算一个数，对账时就会出现"文件与系统不一致"。
func (h *ReceivableHandler) ExportReceivables(c *gin.Context) {
	var q dto.ReceivableQuery
	if !bindQuery(c, &q) {
		return
	}
	list, err := fetchAll(func(page, size int) ([]model.Receivable, int64, error) {
		q.Page, q.PageSize = page, size
		res, err := h.receivables.List(c.Request.Context(), q)
		if err != nil {
			return nil, 0, err
		}
		return res.List, res.Total, nil
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	out := make([][]string, 0, len(list))
	for _, m := range list {
		partner := ""
		if m.Partner != nil {
			partner = m.Partner.Name
		}
		out = append(out, []string{
			m.DocNo, partner, m.SourceDocNo, m.CurrencyCode,
			m.Amount.String(), m.SettledAmount.String(),
			m.Amount.Sub(m.SettledAmount).String(),
			model.DateOnly(m.DueDate).Format("2006-01-02"),
			m.Status.Label(),
		})
	}
	writeCSV(c, "receivables",
		[]string{"单号", "客户", "来源单", "币种", "金额", "已核销", "未结清", "到期日", "状态"}, out)
}

// ExportPayables GET /api/v1/exports/payables
//
// 与应收同构。未结清减掉已开的借项：与账龄、核销校验同一口径。
func (h *PayableHandler) ExportPayables(c *gin.Context) {
	var q dto.PayableQuery
	if !bindQuery(c, &q) {
		return
	}
	list, err := fetchAll(func(page, size int) ([]model.Payable, int64, error) {
		q.Page, q.PageSize = page, size
		res, err := h.payables.List(c.Request.Context(), q)
		if err != nil {
			return nil, 0, err
		}
		return res.List, res.Total, nil
	})
	if err != nil {
		response.Fail(c, err)
		return
	}

	// 借项要减掉（T-52b3）：与账龄、核销校验同一口径，
	// 否则导出的欠款比系统里多，客户拿着文件来问时解释不清。
	ids := make([]uint, 0, len(list))
	for _, m := range list {
		ids = append(ids, m.ID)
	}
	debits, err := h.payables.DebitTotals(c.Request.Context(), ids)
	if err != nil {
		response.Fail(c, err)
		return
	}

	out := make([][]string, 0, len(list))
	for _, m := range list {
		partner := ""
		if m.Partner != nil {
			partner = m.Partner.Name
		}
		outstanding := m.Amount.Sub(m.SettledAmount).Sub(debits[m.ID])
		out = append(out, []string{
			m.DocNo, partner, m.SourceDocNo, m.CurrencyCode,
			m.Amount.String(), m.SettledAmount.String(),
			outstanding.String(),
			model.DateOnly(m.DueDate).Format("2006-01-02"),
			m.Status.Label(),
		})
	}
	writeCSV(c, "payables",
		[]string{"单号", "供应商", "来源单", "币种", "金额", "已付", "未结清", "到期日", "状态"}, out)
}

// ExportSalesOrderChanges GET /api/v1/sales-orders/:id/changes/export
//
// 一张订单的变更历史：一行一个"变了的行"。
//
// 只改交期的变更没有行可展开，用一行空 SKU 占位 —— 否则那次变更在文件里
// 就消失了，而它同样改变了订单。文件名带订单号：同一目录下过两次，
// 靠它才分得清是哪张单的历史。
//
// 不做筛选、也不复用列表接口：变更历史没有分页参数，它天然是"这张单的全部"。
func (h *SalesOrderHandler) ExportSalesOrderChanges(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	order, err := h.orders.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	list, err := h.orders.ListChanges(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}

	rows := orderChangeRows(list)

	writeCSV(c, "order-changes-"+order.DocNo, orderChangeHeader, rows)
}

// orderChangeHeader 是变更历史导出的表头。
//
// 单独提出来，是因为行装配与表头必须同长：少一列会让整份文件错位，
// 而错位的表格比没有表格更难看懂。
var orderChangeHeader = []string{
	"变更序号", "变更单号", "变更时间", "操作人", "原因", "SKU",
	"原数量", "新数量", "原单价", "新单价",
	"原订单总额", "新订单总额", "原交期", "新交期",
}

// orderChangeRows 把变更历史摊成 CSV 行：一行一个"变了的行"。
//
// 抽成纯函数是为了能直接测：这里有两处容易错的地方 ——
// 只改交期的变更没有行可展开（要占位，否则那次变更消失），
// 以及展开之后每一行的列数必须一样（少一列会让整份文件错位）。
func orderChangeRows(list []model.SalesOrderChange) [][]string {
	rows := make([][]string, 0, len(list))
	for _, ch := range list {
		head := []string{
			strconv.Itoa(int(ch.Seq)),
			ch.DocNo,
			ch.CreatedAt.Format("2006-01-02 15:04"),
			ch.Operator,
			ch.Reason,
		}
		tail := []string{
			ch.BeforeTotal.String(),
			ch.AfterTotal.String(),
			dateOrEmpty(ch.BeforeDeliveryDate),
			dateOrEmpty(ch.AfterDeliveryDate),
		}

		if len(ch.Lines) == 0 {
			row := append([]string{}, head...)
			row = append(row, "", "", "", "", "")
			rows = append(rows, append(row, tail...))
			continue
		}
		for _, ln := range ch.Lines {
			row := append([]string{}, head...)
			row = append(row, ln.SKUCode,
				ln.BeforeQty.String(), ln.AfterQty.String(),
				ln.BeforeUnitPrice.String(), ln.AfterUnitPrice.String())
			rows = append(rows, append(row, tail...))
		}
	}
	return rows
}
