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
