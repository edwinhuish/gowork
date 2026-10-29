// Package server 负责组装 gin 引擎与路由表。
//
// 路由集中在这一处定义，方便一眼看清 API 全貌；handler 本身不关心自己被挂在哪个路径下。
package server

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/handler"
	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/server/middleware"
	"github.com/edwinhuish/gowork/internal/service"
)

// Handlers 汇总所有 HTTP handler，由 bootstrap 注入。
type Handlers struct {
	Auth    *handler.AuthHandler
	Product *handler.ProductHandler
	// ProductCode 是产品别名编号（旧 SKU / 客户号 / 供应商货号）。
	ProductCode *handler.ProductCodeHandler
	Partner     *handler.PartnerHandler
	// PartnerAddress 是主体的多地址（发票 / 收货 / 提单收货人 / 通知方）。
	PartnerAddress  *handler.PartnerAddressHandler
	Currency        *handler.CurrencyHandler
	Rate            *handler.ExchangeRateHandler
	Contact         *handler.ContactHandler
	Activity        *handler.ActivityHandler
	Oppty           *handler.OpportunityHandler
	Lead            *handler.LeadHandler
	Source          *handler.ProductSourceHandler
	Quotation       *handler.QuotationHandler
	Contract        *handler.ContractHandler
	PDF             *handler.PDFHandler
	SalesOrder      *handler.SalesOrderHandler
	Purchase        *handler.PurchaseOrderHandler
	PurchaseReceipt *handler.PurchaseReceiptHandler
	Inspection      *handler.InspectionHandler
	Shipment        *handler.ShipmentHandler
	Charge          *handler.ShipmentChargeHandler
	Customs         *handler.CustomsDeclarationHandler
	Receivable      *handler.ReceivableHandler
	CreditNote      *handler.CreditNoteHandler
	DebitNote       *handler.DebitNoteHandler
	Payable         *handler.PayableHandler
	Payment         *handler.PaymentHandler
	Report          *handler.ReportHandler
	Statement       *handler.StatementHandler
	Stock           *handler.StockHandler
	StockDoc        *handler.StockDocumentHandler
	Dashboard       *handler.DashboardHandler
	Attachment      *handler.AttachmentHandler
	Audit           *handler.AuditHandler
	User            *handler.UserHandler
	Clause          *handler.ClauseHandler
}

// bodyLimitBytes 是单个请求体的兜底上限，32MB。
//
// 取值依据：附件业务上限 10MB（service.MaxAttachmentSize），加上 multipart 的
// boundary 与表单字段开销后仍留有充裕余量；而它挡住的是"几 GB 的 POST 把内存吃光"
// 这类请求——那种请求只需要一个 curl 循环，不需要账号（登录接口也在保护范围内）。
const bodyLimitBytes = 32 << 20

// requestTimeout 是单个请求的处理上限。
//
// 取值比导出这类重活宽裕（10000 行分页取数最慢也就几秒），但远小于"用户会一直等"
// 的时长：请求卡住时，早点返回 504 比让浏览器转到超时更好查——日志里会留下
// 一条带 request_id 的超时错误。
const requestTimeout = 30 * time.Second

// New 构建 gin 引擎。
func New(
	cfg *config.Config, log *slog.Logger, auth service.AuthService,
	audit service.AuditService, c cache.Cache, h Handlers,
) *gin.Engine {
	gin.SetMode(ginMode(cfg))

	engine := gin.New()
	engine.Use(
		middleware.RequestID(),
		middleware.AccessLog(log),
		middleware.Recovery(log),
		middleware.CORS(cfg.HTTP.CORSOrigins),
		// 请求体兜底上限：gin 解析 JSON 时会把整个 body 读进内存，而 Go 默认不设限。
		// 附件那 10MB 的业务上限另算（见 BodyLimit 的注释）。
		middleware.BodyLimit(bodyLimitBytes),
		// 请求处理上限：超时由 ctx 取消传导到查库，最终由 response.Fail 报 504。
		middleware.RequestTimeout(requestTimeout),
	)

	engine.GET("/healthz", handler.Health)

	api := engine.Group("/api/v1")

	authGroup := api.Group("/auth")
	{
		authGroup.POST("/login", h.Auth.Login)
		authGroup.GET("/profile", middleware.Auth(auth), h.Auth.Profile)
		authGroup.POST("/logout", middleware.Auth(auth), h.Auth.Logout)
	}

	// 幂等键在鉴权之后：键要按用户隔离，否则 A 的键能回放出 B 的响应。
	// 审计排在最后：它要读身份、也要读最终状态码；而幂等回放在它之前就短路了，
	// 重放的请求因此不会被重复记成两次写操作。
	secured := api.Group("",
		middleware.Auth(auth),
		middleware.Idempotency(c, log),
		middleware.Audit(audit, log),
	)
	{
		secured.GET("/dashboard/summary", h.Dashboard.Summary)

		// 操作日志：只读，且只有管理员能看——里面装着全公司单据的提交内容。
		secured.GET("/audit-logs", middleware.RequireAdmin(), h.Audit.List)

		// 账号目录：只服务"把客户转给谁"的选择器，业务员在 service 里被拒 403。
		secured.GET("/users", h.User.ListActive)
		// 账号维护（T-57）：只有管理员能改。用独立前缀而不是 /users/:id，
		// 因为 gin 同一层不允许静态段与 :id 并存，而 /users 已被选择器占用。
		userAccounts := secured.Group("/user-accounts", middleware.RequireAdmin())
		userAccounts.GET("", h.User.List)
		userAccounts.POST("", h.User.Create)
		userAccounts.PUT("/:id", h.User.Update)

		// 导出（T-35）：参数与各自列表完全一致，导出的就是列表上那一批。
		// 放在 /exports 下而不是 /partners/export：gin 同一层不允许 :id 与静态段并存。
		exports := secured.Group("/exports")
		{
			exports.GET("/partners", h.Partner.Export)
			exports.GET("/products", h.Product.Export)
			// 出货台账（T-63）：参数与列表一致，导出的就是列表上那一批。
			exports.GET("/shipments", h.Shipment.ExportShipments)
			exports.GET("/sales-orders", h.SalesOrder.ExportSalesOrders)
			exports.GET("/payments", h.Payment.ExportPayments)
			exports.GET("/receivables", h.Receivable.ExportReceivables)
			exports.GET("/payables", h.Payable.ExportPayables)
		}

		// 导入（T-35）：先 dry_run 出报告，确认后再落库。导出文件可直接当模板。
		imports := secured.Group("/imports")
		{
			imports.POST("/partners", h.Partner.Import)
		}

		products := secured.Group("/products")
		products.GET("", h.Product.List)
		products.POST("", h.Product.Create)
		products.GET("/:id", h.Product.Get)
		products.PUT("/:id", h.Product.Update)
		products.DELETE("/:id", h.Product.Delete)
		// 删除是可追溯的（软删），恢复就该有入口，否则「删错了」只能靠改库。
		products.POST("/:id/restore", h.Product.Restore)

		// 产品编号：SKU 之外的那些号（旧 SKU、客户物料号、供应商货号、工厂号）。
		// 独立前缀而不是 /products/:id/codes：与货源一致，用 ?product_id= 过滤，
		// 前端按面板取数，不需要为了加一个编号去动产品表单。
		codes := secured.Group("/product-codes")
		codes.GET("", h.ProductCode.List)
		codes.POST("", h.ProductCode.Create)
		codes.DELETE("/:id", h.ProductCode.Delete)

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
		// 恢复与删除对称。只挂这一个入口：恢复只认 id，角色已在记录里。
		partners.POST("/:id/restore", h.Partner.Restore)
		// 信用额度是主体上的商务条件（A5），读它的入口跟着主体走。
		// 只读：额度本身通过 PUT /partners/:id 改。
		partners.GET("/:id/credit", h.Partner.Credit)
		// 合并：:id 是主记录，body 里的 source_id 被并入并软删。不可逆，只挂这一个入口。
		partners.POST("/:id/merge", h.Partner.Merge)
		// 转移负责人：主管把客户分下去，或业务员离职时整批交出去。
		partners.PUT("/:id/owner", h.Partner.TransferOwner)
		// 多地址：单证取址的依据（发票抬头 / 收货人 / 通知方）。
		// 只挂在 /partners 下，不给三个角色切片各复制一份——地址没有角色语义，
		// 能打开这个主体就能看它的地址；复制三份的代价是三处要同步改。
		addresses := partners.Group("/:id/addresses")
		addresses.GET("", h.PartnerAddress.List)
		addresses.POST("", h.PartnerAddress.Create)
		addresses.PUT("/:addressId", h.PartnerAddress.Update)
		addresses.DELETE("/:addressId", h.PartnerAddress.Delete)

		// 查重。单独一个前缀而不是 /partners/duplicates：同一层不能既有 :id
		// 又有静态段（gin 会直接 panic），与 /shipment-candidates 同一处理。
		duplicates := secured.Group("/partner-duplicates")
		duplicates.GET("", h.Partner.Duplicates)
		duplicates.GET("/candidates", h.Partner.DuplicateCandidates)

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

		// 条款模板库（T-27）：主数据，不属于任何一份合同。
		// 名字别与 biz_contract_term（付款分期）混了——两套东西。
		clauses := secured.Group("/clauses")
		clauses.GET("", h.Clause.List)
		clauses.POST("", h.Clause.Create)
		clauses.PUT("/:id", h.Clause.Update)
		clauses.DELETE("/:id", h.Clause.Delete)
		clauses.POST("/:id/restore", h.Clause.Restore)

		// 销售订单：报价谈成后的出货凭证。总额只由明细汇总，状态走完整状态机。
		orders := secured.Group("/sales-orders")
		orders.GET("", h.SalesOrder.List)
		orders.POST("", h.SalesOrder.Create)
		orders.GET("/:id", h.SalesOrder.Get)
		orders.PUT("/:id", h.SalesOrder.Update)
		orders.DELETE("/:id", h.SalesOrder.Delete)
		orders.PUT("/:id/status", h.SalesOrder.UpdateStatus)
		// 已确认订单的变更：不能直接 PUT，改走变更单留痕（T-44）。
		orders.GET("/:id/changes", h.SalesOrder.Changes)
		orders.POST("/:id/changes", h.SalesOrder.Change)
		// 订单打印件（T-54）：客户要一份书面确认时，不必再截图。
		orders.GET("/:id/pdf", h.PDF.SalesOrder)

		// 采购订单：销售订单的镜像，明细可挂到销售订单行做「以销定采」。
		purchases := secured.Group("/purchase-orders")
		purchases.GET("", h.Purchase.List)
		purchases.POST("", h.Purchase.Create)
		purchases.GET("/:id", h.Purchase.Get)
		// 采购跟单节点（T-38）：货在工厂还是已经进仓、验货过没过。
		// 挂在行上，所以按订单取整表、按节点补录。
		purchases.GET("/:id/milestones", h.Purchase.Milestones)
		purchases.PUT("/:id/milestones", h.Purchase.SaveMilestones)
		purchases.PUT("/:id", h.Purchase.Update)
		purchases.DELETE("/:id", h.Purchase.Delete)
		purchases.PUT("/:id/status", h.Purchase.UpdateStatus)
		// 收货单（T-51a）：挂在采购单下——收货永远属于某张采购单。
		purchases.GET("/:id/receipts", h.PurchaseReceipt.Summary)
		purchases.POST("/:id/receipts", h.PurchaseReceipt.Create)

		// 收货单自身：确认才动库存，所以单独一个入口。
		receipts := secured.Group("/purchase-receipts")
		receipts.GET("/:id", h.PurchaseReceipt.Get)
		receipts.PUT("/:id/confirm", h.PurchaseReceipt.Confirm)
		// 退货（T-52b3）：从收货单发起，保证依赖方向 receipt → stockIn。
		receipts.POST("/:id/return", h.PurchaseReceipt.Return)
		receipts.DELETE("/:id", h.PurchaseReceipt.Delete)

		// 验货记录（T-51b）：结论、抽检比例与缺陷项。挂在采购单下，
		// 删除走自己的前缀（独立实体，不再需要采购单 id）。
		purchases.GET("/:id/inspections", h.Inspection.List)
		purchases.POST("/:id/inspections", h.Inspection.Create)
		inspections := secured.Group("/inspections")
		inspections.DELETE("/:id", h.Inspection.Delete)
		// 采购单打印件（T-54）：供应商要拿到它才能排产。
		purchases.GET("/:id/pdf", h.PDF.PurchaseOrder)

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
		// 四份外销单证（T-55）：与 CI / PL 共用套号，只在文件名后缀上区分。
		shipments.GET("/:id/shipping-advice", h.PDF.ShippingAdvice)
		shipments.GET("/:id/certificate-of-origin", h.PDF.CertificateOfOrigin)
		shipments.GET("/:id/insurance-policy", h.PDF.InsurancePolicy)
		shipments.GET("/:id/booking-request", h.PDF.BookingRequest)
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
		// 贷项通知单（T-52b）：冲减某张应收的一部分（退货 / 折让 / 短装）。
		// 借项通知单（T-52b3）：冲减某张应付的一部分，与贷项对称。
		debitNotes := secured.Group("/debit-notes")
		debitNotes.GET("", h.DebitNote.List)
		debitNotes.POST("", h.DebitNote.Create)

		creditNotes := secured.Group("/credit-notes")
		creditNotes.GET("", h.CreditNote.List)
		creditNotes.POST("", h.CreditNote.Create)

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
		// 撤销单条核销（T-52）：一笔核销录错，不该逼人删掉整张收付款单。
		payments.DELETE("/:id/allocations/:allocationId", h.Payment.RemoveAllocation)

		// 报表：跨表聚合，统一走 /reports，不散成各资源后面的 -summary。
		// 账龄按主体给出未核销金额的 30/60/90+ 分桶（T-22）。
		reports := secured.Group("/reports")
		// 客户对账单（T-53）：期初 / 本期发生 / 本期收款 / 期末，逐笔可追到单据。
		reports.GET("/receivable-statement", h.Statement.Receivable)
		// 期末重估（T-60）：未核销外币余额按指定日汇率重算，差额单列。
		reports.GET("/revaluation", h.Report.Revaluation)
		reports.GET("/aging", h.Report.Aging)
		// 单票毛利：收入 − 采购成本 − 运杂费（本币）。?shipment_id= 取单票，供详情页面板用。
		reports.GET("/shipment-profits", h.Report.ShipmentProfits)
		// 汇兑损益：逐笔核销的到账汇率与单据锁定汇率之差，正收益负损失。
		reports.GET("/exchange-gains", h.Report.ExchangeGains)
		// 销售看板：按客户 / 产品 / 国家 / 月汇总订单额与毛利，口径与单票毛利同源。
		reports.GET("/sales-dashboard", h.Report.SalesDashboard)
		// 回款与超期：期间回款率（流量）+ 截至 as_of 的超期明细（存量）。
		reports.GET("/collection", h.Report.Collection)
		// 客户 360：一个主体的订单 / 出货 / 应收 / 跟进 / 商机，各块带 id 可点回源单据。
		reports.GET("/partner-360", h.Report.Partner360)
		// 待办清单：待跟进、应收待催、缺当日汇率，每类给全量条数 + 前 N 条。
		reports.GET("/todos", h.Report.Todos)
		// 产品毛利下钻：某个产品逐票的收入 / 成本 / 运杂费 / 毛利（看板点开用）。
		reports.GET("/product-profit", h.Report.ProductProfit)

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
		// 红冲（T-52c）：生成一张方向相反的单据，库存回到原单确认前。
		stockDocs.POST("/:id/reverse", h.StockDoc.Reverse)

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
