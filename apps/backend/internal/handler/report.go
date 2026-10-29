package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/pkg/response"
	"github.com/edwinhuish/gowork/internal/service"
)

// ReportHandler 是只读报表的 HTTP 入口。
//
// 报表全部挂在 /reports 下，不散在各资源后面当 `-summary`：
// 它们是跨表的聚合，既不属于某张单据，也不跟着某张单据的权限与状态机走。
type ReportHandler struct {
	reports service.ReportService
}

func NewReportHandler(reports service.ReportService) *ReportHandler {
	return &ReportHandler{reports: reports}
}

// Revaluation GET /api/v1/reports/revaluation?direction=&as_of=
//
// 期末重估（T-60）：把未核销的外币余额按指定日汇率重算，差额单列。
// 与汇兑损益分工——后者只覆盖已核销部分。
func (h *ReportHandler) Revaluation(c *gin.Context) {
	var q dto.RevaluationQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.Revaluation(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// ExchangeGains GET /api/v1/reports/exchange-gains
//
// 参数：from / to（按到账日，闭区间，留空不限）、partner_id、currency_code。
func (h *ReportHandler) ExchangeGains(c *gin.Context) {
	var q dto.ExchangeGainQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.ExchangeGains(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// ProductProfit GET /api/v1/reports/product-profit
//
// 参数：product_id（必填）、from / to（按出货日，闭区间）。
// 某产品逐票的收入、成本、摊到的运杂费与毛利，按出货日倒序。
func (h *ReportHandler) ProductProfit(c *gin.Context) {
	var q dto.ProductProfitQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.ProductProfit(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Todos GET /api/v1/reports/todos
//
// 参数：limit（每类行数，留空 20，0 取上限 100）、as_of（"今天"，留空取服务器当天）。
func (h *ReportHandler) Todos(c *gin.Context) {
	var q dto.TodoQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.Todos(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Partner360 GET /api/v1/reports/partner-360
//
// 参数：partner_id（必填）、limit（每块行数，留空 10，0 取上限 50）、
// as_of（超期判断的截止日，留空取今天）。主体不存在返回 404。
func (h *ReportHandler) Partner360(c *gin.Context) {
	var q dto.Partner360Query
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.Partner360(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Collection GET /api/v1/reports/collection
//
// 参数：direction=10|20（留空按应收）、from / to（按到期日，闭区间）、
// as_of（超期明细截止日，留空取今天）、limit（留空 50，0 取上限 500）。
func (h *ReportHandler) Collection(c *gin.Context) {
	var q dto.CollectionQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.Collection(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// SalesDashboard GET /api/v1/reports/sales-dashboard
//
// 参数：dimension=customer|product|country|month（留空按客户）、from / to（闭区间）、
// limit（留空 20，0 取上限 500）。订单额按下单日、毛利按出货日，两个口径并排。
func (h *ReportHandler) SalesDashboard(c *gin.Context) {
	var q dto.SalesDashboardQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.SalesDashboard(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// ShipmentProfits GET /api/v1/reports/shipment-profits
//
// 参数：shipment_id（只取一票）、page / page_size。金额全是本币。
func (h *ReportHandler) ShipmentProfits(c *gin.Context) {
	var q dto.ShipmentProfitQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.ShipmentProfits(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Aging GET /api/v1/reports/aging
//
// 参数：direction=10|20（留空按应收）、partner_id、as_of=YYYY-MM-DD（留空取今天）。
func (h *ReportHandler) Aging(c *gin.Context) {
	var q dto.AgingQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.reports.Aging(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
