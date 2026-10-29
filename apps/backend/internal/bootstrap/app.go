// Package bootstrap 负责依赖装配：把配置、基础设施、仓储、服务、handler 串起来。
//
// 组装逻辑集中在这一处，好处是依赖关系一眼可见，也避免各处 New 满天飞。
// 将来如果依赖复杂度上升，可以平滑替换成 wire 生成。
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/handler"
	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/infra/database"
	"github.com/edwinhuish/gowork/internal/infra/logger"
	"github.com/edwinhuish/gowork/internal/migrate"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/pdf"
	"github.com/edwinhuish/gowork/internal/repository"
	"github.com/edwinhuish/gowork/internal/server"
	"github.com/edwinhuish/gowork/internal/service"
)

// App 持有进程级资源，供 main 启动与优雅退出使用。
type App struct {
	Config *config.Config
	Engine *gin.Engine
	DB     *gorm.DB
	Cache  cache.Cache
	Log    *slog.Logger
}

// New 完成全部装配。任何一步失败都直接返回，避免带病启动。
func New(cfg *config.Config) (*App, error) {
	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(log)

	db, err := database.Open(cfg.Database, log)
	if err != nil {
		return nil, err
	}

	// 表改名必须排在 AutoMigrate 之前。AutoMigrate 只认模型，会在 md_partner
	// 这个名字下直接建一张空表；等它建完再改名就撞名了，而旧表 md_party 里的
	// 数据会被晾在原地 —— 启动照常成功，列表却是空的。
	if err := migrate.RenameLegacyPartnerTable(db, log); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("执行表改名迁移: %w", err)
	}

	// 合并旧主体也必须排在 AutoMigrate 之前。AutoMigrate 会按模型给
	// md_product.supplier_id 建指向 md_partner 的外键，而老库存的是 md_supplier
	// 的 id——外键一建，那些行立刻违反约束，整个升级失败。
	// 先在空表上完成搬运与 id 重映射，外键建出来时数据才是自洽的。
	if err := migrate.PreparePartnerTable(db); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("准备 md_partner: %w", err)
	}
	if err := migrate.MergeLegacyCustomersAndSuppliers(db, log); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("合并旧客户与供应商: %w", err)
	}

	// 全部业务表集中在这里声明一次：结构同步、软删标记改造、以及将来的
	// 备份与迁移都读同一份清单，分开写必然漏表。
	models := []any{
		&model.User{},
		&model.Partner{},
		&model.PartnerAddress{},
		&model.Product{},
		&model.ProductCode{},
		&model.Currency{},
		&model.ExchangeRate{},
		&model.ProductSource{},
		&model.DocSeq{},
		&model.Contact{},
		&model.Activity{},
		&model.Opportunity{},
		&model.Lead{},
		&model.Quotation{},
		&model.QuotationItem{},
		&model.Contract{},
		&model.ContractTerm{},
		&model.ContractClause{},
		&model.Clause{},
		&model.DocNoRename{},
		&model.SalesOrder{},
		&model.SalesOrderItem{},
		&model.SalesOrderChange{},
		&model.SalesOrderChangeLine{},
		&model.PurchaseOrder{},
		&model.PurchaseOrderItem{},
		&model.Shipment{},
		&model.ShipmentAllocation{},
		&model.ShipmentCharge{},
		&model.CustomsDeclaration{},
		&model.Receivable{},
		&model.Payable{},
		&model.Payment{},
		&model.PaymentAllocation{},
		&model.StockMovement{},
		&model.StockSnapshot{},
		&model.StockDocument{},
		&model.StockDocumentItem{},
		&model.Attachment{},
		&model.AttachmentContent{},
		&model.AuditLog{},
	}

	if cfg.Database.AutoMigrate {
		// 软删标记的列改造藏在 AutoMigrate 里：任何调用点都会得到同一条升级路径，
		// 不会因为某个入口漏调而升级到一半炸开。
		if err := database.AutoMigrate(db, models...); err != nil {
			_ = database.Close(db)
			return nil, err
		}
		log.Info("数据库结构已同步")
	}

	// 数据迁移必须排在结构同步之后：它要把旧表的数据读出来写进新表，
	// 而新表是上一步才建出来的。顺序反了就是"表不存在"。
	if err := migrate.Run(db, log); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("执行数据迁移: %w", err)
	}

	c, err := cache.New(cfg.Cache, log)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("初始化缓存: %w", err)
	}

	// ---- 数据访问层 ----
	userRepo := repository.NewUserRepository(db)
	partnerRepo := repository.NewPartnerRepository(db)
	productRepo := repository.NewProductRepository(db)
	productCodeRepo := repository.NewProductCodeRepository(db)
	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	sourceRepo := repository.NewProductSourceRepository(db)
	contactRepo := repository.NewContactRepository(db)
	activityRepo := repository.NewActivityRepository(db)
	opptyRepo := repository.NewOpportunityRepository(db)
	leadRepo := repository.NewLeadRepository(db)
	quotationRepo := repository.NewQuotationRepository(db)
	contractRepo := repository.NewContractRepository(db)
	clauseRepo := repository.NewClauseRepository(db)
	orderRepo := repository.NewSalesOrderRepository(db)
	purchaseRepo := repository.NewPurchaseOrderRepository(db)
	// 采购跟单节点（T-38）：挂在采购订单行上，随详情一起读、单独补录。
	purchaseMilestoneRepo := repository.NewPurchaseMilestoneRepository(db)
	shipmentRepo := repository.NewShipmentRepository(db)
	chargeRepo := repository.NewShipmentChargeRepository(db)
	customsRepo := repository.NewCustomsDeclarationRepository(db)
	receivableRepo := repository.NewReceivableRepository(db)
	payableRepo := repository.NewPayableRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	reportRepo := repository.NewReportRepository(db)
	stockRepo := repository.NewStockRepository(db)
	stockDocRepo := repository.NewStockDocumentRepository(db)
	attachmentRepo := repository.NewAttachmentRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	// 事务边界只有一个出入口：跨仓储的写入走它，仓储按 ctx 里的连接自动加入。
	txs := repository.NewTxManager(db)

	// ---- 业务层 ----
	authSvc := service.NewAuthService(
		userRepo,
		c,
		service.AuthConfig{
			Secret:       cfg.JWT.Secret,
			Expire:       time.Duration(cfg.JWT.Expire) * time.Hour,
			MaxLoginFail: 5,
		},
		service.WithAuthLogger(log),
	)
	productSvc := service.NewProductService(
		productRepo, partnerRepo, sourceRepo, productCodeRepo, txs, service.WithProductLogger(log))
	// 编号自己一套读写：它是产品的别名，不是产品的一个字段，
	// 前端在产品详情页当成一个面板用，产品表单不参与。
	productCodeSvc := service.NewProductCodeService(
		productCodeRepo, productRepo, service.WithProductCodeLogger(log))
	partnerSvc := service.NewPartnerService(
		partnerRepo, productRepo, currencyRepo, userRepo, service.WithPartnerLogger(log))
	// 多地址跟着主体走：只依赖主体仓储（要确认主体存在），不依赖任何业务服务。
	partnerAddressSvc := service.NewPartnerAddressService(
		repository.NewPartnerAddressRepository(db), partnerRepo,
		service.WithPartnerAddressLogger(log))
	currencySvc := service.NewCurrencyService(currencyRepo, rateRepo, service.WithCurrencyLogger(log))
	rateSvc := service.NewExchangeRateService(rateRepo, currencyRepo, service.WithExchangeRateLogger(log))
	contactSvc := service.NewContactService(contactRepo, partnerRepo, service.WithContactLogger(log))
	activitySvc := service.NewActivityService(
		activityRepo, partnerRepo, contactRepo, service.WithActivityLogger(log))
	opptySvc := service.NewOpportunityService(
		opptyRepo, partnerRepo, currencySvc, service.WithOpportunityLogger(log))
	leadSvc := service.NewLeadService(
		leadRepo, partnerSvc, partnerRepo, txs, service.WithLeadLogger(log))
	sourceSvc := service.NewProductSourceService(
		sourceRepo, productRepo, partnerRepo, currencySvc, service.WithProductSourceLogger(log))
	// 单号服务在这里落地：T-10 是第一张真正需要 DocNo 的单据。
	docNoSvc := service.NewDocNoService(
		repository.NewDocSeqRepository(db), service.WithDocNoLogger(log))
	// 信用额度读订单与应收两张表算敞口（T-39）：它只被订单确认用到，
	// 但要先于订单服务建好。
	creditSvc := service.NewCreditService(
		repository.NewCreditRepository(db), partnerRepo, service.WithCreditLogger(log))
	orderSvc := service.NewSalesOrderService(
		orderRepo, partnerRepo, productRepo, shipmentRepo, currencySvc, docNoSvc, creditSvc,
		service.WithSalesOrderLogger(log),
		service.WithSalesOrderChanges(repository.NewSalesOrderChangeRepository(db)))
	// 报价单要在转换时建订单，所以它依赖销售订单服务；顺序不能反。
	quotationSvc := service.NewQuotationService(
		quotationRepo, partnerRepo, productRepo, sourceRepo, currencySvc, docNoSvc, orderSvc, txs,
		service.WithQuotationLogger(log))
	// 合同只从报价单或订单抄快照，读的是两边仓储而不是两个服务：
	// 它要的是"那张单现在是什么状态、金额多少"，不是"能不能改"。
	contractSvc := service.NewContractService(
		contractRepo, quotationRepo, orderRepo, currencySvc, docNoSvc, partnerRepo,
		clauseRepo, service.WithContractLogger(log))
	// 条款模板库（T-27）：主数据，只被合同生成时读取，不被任何业务流转依赖。
	clauseSvc := service.NewClauseService(clauseRepo, service.WithClauseLogger(log))

	// 打印是附加能力：缺中文字体或模板写坏只降级，不让服务起不来。
	pdfSvc := buildPDFService(
		cfg, log, quotationRepo, orderRepo, contractRepo, shipmentRepo, currencySvc,
		repository.NewPartnerAddressRepository(db))
	purchaseSvc := service.NewPurchaseOrderService(
		purchaseRepo, partnerRepo, productRepo, sourceRepo, orderRepo, purchaseMilestoneRepo,
		currencySvc, docNoSvc,
		service.WithPurchaseOrderLogger(log))
	// 运费杂费只依赖出货仓储（确认这票存在）与币种服务（折本币），
	// 不依赖出货服务：它要的是"这票还在不在"，不是"这票能不能改"。
	chargeSvc := service.NewShipmentChargeService(
		chargeRepo, shipmentRepo, partnerRepo, currencySvc,
		service.WithShipmentChargeLogger(log))
	customsSvc := service.NewCustomsDeclarationService(
		customsRepo, shipmentRepo, service.WithCustomsDeclarationLogger(log))
	// 应收应付各持自己的订单仓储：销售单上取客户与销售币种，
	// 采购单上取供应商与采购币种，两边汇率互不影响。
	receivableSvc := service.NewReceivableService(
		receivableRepo, orderRepo, partnerRepo, docNoSvc,
		service.WithReceivableLogger(log))
	payableSvc := service.NewPayableService(
		payableRepo, purchaseRepo, partnerRepo, docNoSvc,
		service.WithPayableLogger(log))
	// 收付款单读应收 / 应付仓储只为了填核销行与给出可读的错误，
	// 真正的额度判定在仓储事务里做，所以这里依赖仓储而不是两个服务。
	paymentSvc := service.NewPaymentService(
		paymentRepo, receivableRepo, payableRepo, currencySvc, docNoSvc, partnerRepo,
		service.WithPaymentLogger(log))
	// 库存只依赖流水仓储与产品（取快照用的 SKU / 品名），不依赖任何单据服务：
	// 单据产生流水的那一步由 T-16 的单据层调用，方向始终是"单据 → 流水"。
	stockSvc := service.NewStockService(
		stockRepo, productRepo, docNoSvc, service.WithStockLogger(log))
	// 出入库单确认时自己写流水（走 StockDocumentRepository 的一个事务），
	// 不依赖 StockService：那会让"单据 → 流水"绕一圈变成服务间互调。
	stockDocSvc := service.NewStockDocumentService(
		stockDocRepo, productRepo, partnerRepo, docNoSvc,
		service.WithStockDocumentLogger(log))
	// 出货批次管的是"订单行已经被出掉多少"，所以它依赖销售订单仓储（读行与状态），
	// 而不是依赖销售订单服务——服务层之间互相调用会让数量校验绕两圈才算得清。
	// 出货确认会调 StockDocumentService 出一张销售出库单（T-40），
	// 所以它必须排在这之后装配。
	shipmentSvc := service.NewShipmentService(
		shipmentRepo, orderRepo, chargeRepo, customsRepo, docNoSvc, txs, stockDocSvc,
		service.WithShipmentLogger(log))
	// 附件不依赖任何业务服务：它能挂在哪由仓储里的那张清单判定，
	// 加一个可挂载的实体不用改这里的装配。
	attachmentSvc := service.NewAttachmentService(
		attachmentRepo, service.WithAttachmentLogger(log))
	// 报表只读不写，也不缓存：数字要能和明细逐笔对上。
	reportSvc := service.NewReportService(reportRepo, service.WithReportLogger(log))
	dashboardSvc := service.NewDashboardService(productRepo, partnerRepo, c, service.WithDashboardLogger(log))
	// 审计服务只被中间件写、被一个只读接口读：业务代码不调用它，
	// 所以它不挂在任何业务服务上。
	auditSvc := service.NewAuditService(auditRepo, service.WithAuditLogger(log))
	// 账号目录只有"把客户转给谁"这一个用途，所以只暴露一个只读列表。
	userSvc := service.NewUserService(userRepo)

	// ---- HTTP 层 ----
	engine := server.New(cfg, log, authSvc, auditSvc, c, server.Handlers{
		Auth:           handler.NewAuthHandler(authSvc),
		Product:        handler.NewProductHandler(productSvc),
		ProductCode:    handler.NewProductCodeHandler(productCodeSvc),
		Partner:        handler.NewPartnerHandler(partnerSvc, creditSvc),
		PartnerAddress: handler.NewPartnerAddressHandler(partnerAddressSvc),
		Currency:       handler.NewCurrencyHandler(currencySvc),
		Rate:           handler.NewExchangeRateHandler(rateSvc),
		Contact:        handler.NewContactHandler(contactSvc),
		Activity:       handler.NewActivityHandler(activitySvc),
		Oppty:          handler.NewOpportunityHandler(opptySvc),
		Lead:           handler.NewLeadHandler(leadSvc),
		Source:         handler.NewProductSourceHandler(sourceSvc),
		Quotation:      handler.NewQuotationHandler(quotationSvc),
		Contract:       handler.NewContractHandler(contractSvc),
		PDF:            handler.NewPDFHandler(pdfSvc),
		SalesOrder:     handler.NewSalesOrderHandler(orderSvc),
		Purchase:       handler.NewPurchaseOrderHandler(purchaseSvc),
		Shipment:       handler.NewShipmentHandler(shipmentSvc),
		Charge:         handler.NewShipmentChargeHandler(chargeSvc),
		Customs:        handler.NewCustomsDeclarationHandler(customsSvc),
		Receivable:     handler.NewReceivableHandler(receivableSvc),
		Payable:        handler.NewPayableHandler(payableSvc),
		Payment:        handler.NewPaymentHandler(paymentSvc),
		Report:         handler.NewReportHandler(reportSvc),
		Stock:          handler.NewStockHandler(stockSvc),
		StockDoc:       handler.NewStockDocumentHandler(stockDocSvc),
		Dashboard:      handler.NewDashboardHandler(dashboardSvc),
		Attachment:     handler.NewAttachmentHandler(attachmentSvc),
		Audit:          handler.NewAuditHandler(auditSvc),
		User:           handler.NewUserHandler(userSvc),
		Clause:         handler.NewClauseHandler(clauseSvc),
	})

	app := &App{Config: cfg, Engine: engine, DB: db, Cache: c, Log: log}

	if cfg.Database.Seed {
		if err := app.seed(context.Background(), authSvc,
			userRepo, partnerRepo, productRepo, currencyRepo, rateRepo); err != nil {
			app.Close()
			return nil, fmt.Errorf("初始化种子数据: %w", err)
		}
	}

	return app, nil
}

// Close 释放进程级资源，按依赖倒序关闭。
func (a *App) Close() {
	if a.Cache != nil {
		if err := a.Cache.Close(); err != nil {
			a.Log.Warn("关闭缓存出错", "err", err)
		}
	}
	if a.DB != nil {
		if err := database.Close(a.DB); err != nil {
			a.Log.Warn("关闭数据库出错", "err", err)
		}
	}
}

// buildPDFService 装配打印能力。
//
// 字体与模板都是可选的：缺字体时打印不可用（PDF 接口会明确说缺什么），
// 但服务照常启动——报价、合同、出货都不依赖打印这件事。
func buildPDFService(
	cfg *config.Config,
	log *slog.Logger,
	quotations repository.QuotationRepository,
	orders repository.SalesOrderRepository,
	contracts repository.ContractRepository,
	shipments repository.ShipmentRepository,
	currencies service.CurrencyService,
	addresses repository.PartnerAddressRepository,
) service.PDFService {
	fontPath := cfg.PDF.FontPath
	if fontPath == "" {
		fontPath = pdf.DetectFont()
	}
	if fontPath == "" {
		// 不是错误：很多精简镜像里没有中文字体，为了打印让整个服务起不来不值得。
		log.Warn("未找到中文字体，单据 PDF 不可用；用 pdf.font_path 指定一个 .ttf")
		return nil
	}

	set, err := pdf.Load(cfg.PDF.TemplateDir)
	if err != nil {
		log.Warn("PDF 模板不可用，单据 PDF 不可用", "err", err)
		return nil
	}

	renderer, err := pdf.NewRenderer(set, fontPath)
	if err != nil {
		log.Warn("PDF 字体加载失败，单据 PDF 不可用", "err", err, "font", fontPath)
		return nil
	}

	log.Info("单据 PDF 已启用", "font", fontPath, "templates", set.Names())
	return service.NewPDFService(quotations, orders, contracts, shipments, currencies, renderer,
		service.CompanyInfo{
			Name:    cfg.PDF.CompanyName,
			Address: cfg.PDF.CompanyAddress,
			Contact: cfg.PDF.CompanyContact,
		},
		service.WithPDFLogger(log),
		// 多地址取址（T-49）：CI 与 PL 按用途取发票 / 收货 / 通知方地址。
		// 取不到时 PDF 服务自己回退主体上的那一列，所以这里不必判空。
		service.WithPDFAddresses(addresses),
	)
}
