// Package server 负责组装 gin 引擎与路由表。
//
// 路由集中在这一处定义，方便一眼看清 API 全貌；handler 本身不关心自己被挂在哪个路径下。
package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/handler"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/server/middleware"
	"github.com/edwinhuish/gowork/internal/service"
)

// Handlers 汇总所有 HTTP handler，由 bootstrap 注入。
type Handlers struct {
	Auth       *handler.AuthHandler
	Product    *handler.ProductHandler
	Partner    *handler.PartnerHandler
	Currency   *handler.CurrencyHandler
	Rate       *handler.ExchangeRateHandler
	Contact    *handler.ContactHandler
	Activity   *handler.ActivityHandler
	Oppty      *handler.OpportunityHandler
	Lead       *handler.LeadHandler
	Source     *handler.ProductSourceHandler
	Quotation  *handler.QuotationHandler
	Contract   *handler.ContractHandler
	PDF        *handler.PDFHandler
	SalesOrder *handler.SalesOrderHandler
	Purchase   *handler.PurchaseOrderHandler
	Shipment   *handler.ShipmentHandler
	Charge     *handler.ShipmentChargeHandler
	Customs    *handler.CustomsDeclarationHandler
	Receivable *handler.ReceivableHandler
	Payable    *handler.PayableHandler
	Payment    *handler.PaymentHandler
	Report     *handler.ReportHandler
	Stock      *handler.StockHandler
	StockDoc   *handler.StockDocumentHandler
	Dashboard  *handler.DashboardHandler
	Attachment *handler.AttachmentHandler
}

// New 构建 gin 引擎。
func New(cfg *config.Config, log *slog.Logger, auth service.AuthService, h Handlers) *gin.Engine {
	gin.SetMode(ginMode(cfg))

	engine := gin.New()
	engine.Use(
		middleware.RequestID(),
		middleware.AccessLog(log),
		middleware.Recovery(log),
		middleware.CORS(cfg.HTTP.CORSOrigins),
	)

	engine.GET("/healthz", handler.Health)

	api := engine.Group("/api/v1")

	authGroup := api.Group("/auth")
	{
		authGroup.POST("/login", h.Auth.Login)
		authGroup.GET("/profile", middleware.Auth(auth), h.Auth.Profile)
		authGroup.POST("/logout", middleware.Auth(auth), h.Auth.Logout)
	}

	secured := api.Group("", middleware.Auth(auth))
	{
		secured.GET("/dashboard/summary", h.Dashboard.Summary)

		products := secured.Group("/products")
		products.GET("", h.Product.List)
		products.POST("", h.Product.Create)
		products.GET("/:id", h.Product.Get)
		products.PUT("/:id", h.Product.Update)
		products.DELETE("/:id", h.Product.Delete)

		// 产品货源：一个产品对多个供应商的货号与采购价，报价单的成本从这里取。
		// 用 ?product_id= 过滤，与产品详情页的货源面板一一对应。
		sources := secured.Group("/product-sources")
		sources.GET("", h.Source.List)
		sources.POST("", h.Source.Create)
		sources.GET("/:id", h.Source.Get)
		sources.PUT("/:id", h.Source.Update)
		sources.DELETE("/:id", h.Source.Delete)

		partners := secured.Group("/partners")
		partners.GET("", h.Partner.List(nil))
		partners.POST("", h.Partner.Create(nil))
		partners.GET("/:id", h.Partner.Get(nil))
		partners.PUT("/:id", h.Partner.Update(nil))
		partners.DELETE("/:id", h.Partner.Delete(nil))
		// 信用额度是主体上的商务条件（A5），读它的入口跟着主体走。
		// 只读：额度本身通过 PUT /partners/:id 改。
		partners.GET("/:id/credit", h.Partner.Credit)

		// 角色切片入口。客户、供应商与货代已经合成一张表，但前端页面按 resource
		// 名拼 URL，且外部可能存着 /customers/12 这类链接。这三组路由把同一份数据
		// 按角色切开暴露，行为与合并前一致。
		// 常量不能取地址，先落成变量再传指针。
		customerRole := model.PartnerRoleCustomer
		supplierRole := model.PartnerRoleSupplier
		forwarderRole := model.PartnerRoleForwarder
		customers := secured.Group("/customers")
		customers.GET("", h.Partner.List(&customerRole))
		customers.POST("", h.Partner.Create(&customerRole))
		customers.GET("/:id", h.Partner.Get(&customerRole))
		customers.PUT("/:id", h.Partner.Update(&customerRole))
		customers.DELETE("/:id", h.Partner.Delete(&customerRole))

		suppliers := secured.Group("/suppliers")
		suppliers.GET("", h.Partner.List(&supplierRole))
		suppliers.POST("", h.Partner.Create(&supplierRole))
		suppliers.GET("/:id", h.Partner.Get(&supplierRole))
		suppliers.PUT("/:id", h.Partner.Update(&supplierRole))
		suppliers.DELETE("/:id", h.Partner.Delete(&supplierRole))

		forwarders := secured.Group("/forwarders")
		forwarders.GET("", h.Partner.List(&forwarderRole))
		forwarders.POST("", h.Partner.Create(&forwarderRole))
		forwarders.GET("/:id", h.Partner.Get(&forwarderRole))
		forwarders.PUT("/:id", h.Partner.Update(&forwarderRole))
		forwarders.DELETE("/:id", h.Partner.Delete(&forwarderRole))

		// 币种与汇率：后面每张单据都引用它们（币种代码 + 当日汇率）。
		currencies := secured.Group("/currencies")
		currencies.GET("", h.Currency.List)
		currencies.POST("", h.Currency.Create)
		currencies.GET("/:id", h.Currency.Get)
		currencies.PUT("/:id", h.Currency.Update)
		currencies.DELETE("/:id", h.Currency.Delete)

		// 汇率没有 PUT：历史汇率不可改，录错就删掉重录。
		rates := secured.Group("/exchange-rates")
		rates.GET("", h.Rate.List)
		rates.POST("", h.Rate.Create)
		rates.GET("/:id", h.Rate.Get)
		rates.DELETE("/:id", h.Rate.Delete)

		// CRM 侧第一个模块。联系人挂在往来单位下，用 partner_id 过滤。
		contacts := secured.Group("/contacts")
		contacts.GET("", h.Contact.List)
		contacts.POST("", h.Contact.Create)
		contacts.GET("/:id", h.Contact.Get)
		contacts.PUT("/:id", h.Contact.Update)
		contacts.DELETE("/:id", h.Contact.Delete)

		// 跟进记录：时间线按 occurred_at 倒序，?due=1 查已到期未跟进。
		activities := secured.Group("/activities")
		activities.GET("", h.Activity.List)
		activities.POST("", h.Activity.Create)
		activities.GET("/:id", h.Activity.Get)
		activities.PUT("/:id", h.Activity.Update)
		activities.DELETE("/:id", h.Activity.Delete)

		// 商机：列表按预计成交日升序；汇总走独立前缀，避免与 :id 抢路径。
		opportunities := secured.Group("/opportunities")
		opportunities.GET("", h.Oppty.List)
		opportunities.POST("", h.Oppty.Create)
		opportunities.GET("/:id", h.Oppty.Get)
		opportunities.PUT("/:id", h.Oppty.Update)
		opportunities.DELETE("/:id", h.Oppty.Delete)

		secured.GET("/opportunities-summary", h.Oppty.Summary)

		// 线索：转化是独立动作，不能靠改 status 完成。
		leads := secured.Group("/leads")
		leads.GET("", h.Lead.List)
		leads.POST("", h.Lead.Create)
		leads.GET("/:id", h.Lead.Get)
		leads.PUT("/:id", h.Lead.Update)
		leads.DELETE("/:id", h.Lead.Delete)
		leads.POST("/:id/convert", h.Lead.Convert)

		// 报价单：主链上第一张真正的单据。单号走 DocNoService，状态走状态机。
		// 状态变更与另存新版本各自独立成接口，避免"保存一下顺手把报价确认了"。
		quotations := secured.Group("/quotations")
		quotations.GET("", h.Quotation.List)
		quotations.POST("", h.Quotation.Create)
		quotations.GET("/:id", h.Quotation.Get)
		quotations.PUT("/:id", h.Quotation.Update)
		quotations.DELETE("/:id", h.Quotation.Delete)
		quotations.PUT("/:id/status", h.Quotation.UpdateStatus)
		quotations.POST("/:id/versions", h.Quotation.NewVersion)
		// 报价转订单：一次点击生成订单草稿，之后这张报价单不能再转。
		quotations.POST("/:id/convert", h.Quotation.Convert)
		// 打印走 GET：PDF 是附件，不能进统一响应体，错误仍走统一体。
		quotations.GET("/:id/pdf", h.PDF.Quotation)

		// 试算独立前缀：与 /:id 同级会抢路径（同 opportunities-summary 的处理）。
		secured.POST("/quotations-preview", h.Quotation.Preview)

		// 合同：形式发票（PI）与销售合同共用一张表，只能从报价单或订单生成。
		// 没有「空白新建」——金额、币种、汇率、客户全部照抄来源单据，
		// 分段付款的条款随单提交，合计必须等于来源单据的总额。
		contracts := secured.Group("/contracts")
		contracts.GET("", h.Contract.List)
		contracts.POST("", h.Contract.Generate)
		contracts.GET("/:id", h.Contract.Get)
		contracts.PUT("/:id", h.Contract.Update)
		contracts.DELETE("/:id", h.Contract.Delete)
		contracts.PUT("/:id/status", h.Contract.UpdateStatus)
		contracts.GET("/:id/pdf", h.PDF.Contract)

		// 销售订单：报价谈成后的出货凭证。总额只由明细汇总，状态走完整状态机。
		orders := secured.Group("/sales-orders")
		orders.GET("", h.SalesOrder.List)
		orders.POST("", h.SalesOrder.Create)
		orders.GET("/:id", h.SalesOrder.Get)
		orders.PUT("/:id", h.SalesOrder.Update)
		orders.DELETE("/:id", h.SalesOrder.Delete)
		orders.PUT("/:id/status", h.SalesOrder.UpdateStatus)

		// 采购订单：销售订单的镜像，明细可挂到销售订单行做「以销定采」。
		purchases := secured.Group("/purchase-orders")
		purchases.GET("", h.Purchase.List)
		purchases.POST("", h.Purchase.Create)
		purchases.GET("/:id", h.Purchase.Get)
		purchases.PUT("/:id", h.Purchase.Update)
		purchases.DELETE("/:id", h.Purchase.Delete)
		purchases.PUT("/:id/status", h.Purchase.UpdateStatus)

		// 出货批次：一票装哪些销售订单行、各多少。数量校验在服务端做
		// （不得超过该行未出数量），界面上拦不住两个页面同时开着的情况。
		shipments := secured.Group("/shipments")
		shipments.GET("", h.Shipment.List)
		shipments.POST("", h.Shipment.Create)
		shipments.GET("/:id", h.Shipment.Get)
		shipments.PUT("/:id", h.Shipment.Update)
		shipments.DELETE("/:id", h.Shipment.Delete)
		shipments.PUT("/:id/status", h.Shipment.UpdateStatus)
		// 物流节点单独一个入口：提单号与船期订舱后才有，那时单子已经不可改了。
		shipments.PUT("/:id/logistics", h.Shipment.UpdateLogistics)
		// 商业发票与装箱单不单独发号：它们印的是送货单的套号（DN-2610-001），
		// 只是文件名后缀不同（-CI / -PL）。所以没有"新建商票"这种动作，只有打印。
		shipments.GET("/:id/invoice", h.PDF.Invoice)
		shipments.GET("/:id/packing-list", h.PDF.PackingList)
		// 箱唛也是一箱一贴的打印件，不发号：主标识印销售订单号，发票号只是贴纸位。
		shipments.GET("/:id/carton-labels", h.PDF.CartonLabels)
		// 改证重开：号码不变、版本加一。它是"重新签发"，不是"另开一票"。
		shipments.POST("/:id/reissue", h.Shipment.Reissue)

		// 可出货的订单行：出货单新建页的选行来源。独立前缀，避免与 /:id 抢路径。
		secured.GET("/shipment-candidates", h.Shipment.Candidates)

		// 运费杂费：归集到一票货的成本。不校验出货状态——货代账单总是晚于出货到达。
		charges := secured.Group("/shipment-charges")
		charges.GET("", h.Charge.List)
		charges.POST("", h.Charge.Create)
		charges.GET("/:id", h.Charge.Get)
		charges.PUT("/:id", h.Charge.Update)
		charges.DELETE("/:id", h.Charge.Delete)

		// 一票货的费用合计。独立前缀，理由同上。
		secured.GET("/shipment-charges-summary", h.Charge.Summary)

		// 报关单：一票货可以有多张（分票申报、改单重报）。
		// 报关单号与提单号都是外部号，只记录、不参与发号与唯一性。
		customs := secured.Group("/customs-declarations")
		customs.GET("", h.Customs.List)
		customs.POST("", h.Customs.Create)
		customs.GET("/:id", h.Customs.Get)
		customs.PUT("/:id", h.Customs.Update)
		customs.DELETE("/:id", h.Customs.Delete)

		// 应收 / 应付：由订单生成，币种与汇率从来源单据锁定（蓝图 F3 的硬规则）。
		// 一期一张单，核销（T-21）按单进行。
		receivables := secured.Group("/receivables")
		receivables.GET("", h.Receivable.List)
		receivables.POST("", h.Receivable.Create)
		receivables.GET("/:id", h.Receivable.Get)
		receivables.PUT("/:id", h.Receivable.Update)
		receivables.DELETE("/:id", h.Receivable.Delete)

		payables := secured.Group("/payables")
		payables.GET("", h.Payable.List)
		payables.POST("", h.Payable.Create)
		payables.GET("/:id", h.Payable.Get)
		payables.PUT("/:id", h.Payable.Update)
		payables.DELETE("/:id", h.Payable.Delete)

		// 收付款单：核销行随单创建、随单删除，所以没有单独的子资源路由。
		// 方向（收 / 付）决定能核应收还是应付，汇率取到账日的。
		payments := secured.Group("/payments")
		payments.GET("", h.Payment.List)
		payments.POST("", h.Payment.Create)
		payments.GET("/:id", h.Payment.Get)
		payments.PUT("/:id", h.Payment.Update)
		payments.DELETE("/:id", h.Payment.Delete)

		// 报表：跨表聚合，统一走 /reports，不散成各资源后面的 -summary。
		// 账龄按主体给出未核销金额的 30/60/90+ 分桶（T-22）。
		reports := secured.Group("/reports")
		reports.GET("/aging", h.Report.Aging)
		// 单票毛利：收入 − 采购成本 − 运杂费（本币）。?shipment_id= 取单票，供详情页面板用。
		reports.GET("/shipment-profits", h.Report.ShipmentProfits)
		// 汇兑损益：逐笔核销的到账汇率与单据锁定汇率之差，正收益负损失。
		reports.GET("/exchange-gains", h.Report.ExchangeGains)

		movements := secured.Group("/stock-movements")
		movements.GET("", h.Stock.ListMovements)
		movements.POST("", h.Stock.CreateMovement)
		movements.GET("/:id", h.Stock.GetMovement)
		movements.DELETE("/:id", h.Stock.DeleteMovement)

		// 快照只读 + 重算。没有 PUT：能改库存的只有流水。
		snapshots := secured.Group("/stock-snapshots")
		snapshots.GET("", h.Stock.ListSnapshots)
		snapshots.POST("/recalc", h.Stock.Recalc)

		// 出入库单：确认才产生流水，所以状态推进单独一个动作路由。
		stockDocs := secured.Group("/stock-documents")
		stockDocs.GET("", h.StockDoc.List)
		stockDocs.POST("", h.StockDoc.Create)
		stockDocs.GET("/:id", h.StockDoc.Get)
		stockDocs.PUT("/:id", h.StockDoc.Update)
		stockDocs.DELETE("/:id", h.StockDoc.Delete)
		stockDocs.POST("/:id/confirm", h.StockDoc.Confirm)

		// 附件：横切的归档能力，不挂在任何业务前缀下——挂什么由 entity_type 决定。
		// 列表与上传共用同一个入口，且 entity_type + entity_id 都是必填：
		// 不带条件的列表等于把全库的提单摊在一个接口上。
		attachments := secured.Group("/attachments")
		attachments.GET("", h.Attachment.List)
		attachments.POST("", h.Attachment.Upload)
		attachments.GET("/:id/download", h.Attachment.Download)
		attachments.DELETE("/:id", h.Attachment.Delete)
	}

	return engine
}

func ginMode(cfg *config.Config) string {
	if cfg.HTTP.Mode == "release" || cfg.IsProd() {
		return gin.ReleaseMode
	}
	return gin.DebugMode
}
