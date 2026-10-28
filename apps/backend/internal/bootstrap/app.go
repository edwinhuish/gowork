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

	if cfg.Database.AutoMigrate {
		if err := database.AutoMigrate(db,
			&model.User{},
			&model.Partner{},
			&model.Product{},
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
			&model.DocNoRename{},
			&model.SalesOrder{},
			&model.SalesOrderItem{},
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
		); err != nil {
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
	currencyRepo := repository.NewCurrencyRepository(db)
	rateRepo := repository.NewExchangeRateRepository(db)
	sourceRepo := repository.NewProductSourceRepository(db)
	contactRepo := repository.NewContactRepository(db)
	activityRepo := repository.NewActivityRepository(db)
	opptyRepo := repository.NewOpportunityRepository(db)
	leadRepo := repository.NewLeadRepository(db)
	quotationRepo := repository.NewQuotationRepository(db)
	contractRepo := repository.NewContractRepository(db)
	orderRepo := repository.NewSalesOrderRepository(db)
	purchaseRepo := repository.NewPurchaseOrderRepository(db)
	shipmentRepo := repository.NewShipmentRepository(db)
	chargeRepo := repository.NewShipmentChargeRepository(db)
	customsRepo := repository.NewCustomsDeclarationRepository(db)
	receivableRepo := repository.NewReceivableRepository(db)
	payableRepo := repository.NewPayableRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	reportRepo := repository.NewReportRepository(db)
	stockRepo := repository.NewStockRepository(db)
	stockDocRepo := repository.NewStockDocumentRepository(db)

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
		productRepo, partnerRepo, sourceRepo, service.WithProductLogger(log))
	partnerSvc := service.NewPartnerService(
		partnerRepo, productRepo, currencyRepo, service.WithPartnerLogger(log))
	currencySvc := service.NewCurrencyService(currencyRepo, rateRepo, service.WithCurrencyLogger(log))
	rateSvc := service.NewExchangeRateService(rateRepo, currencyRepo, service.WithExchangeRateLogger(log))
	contactSvc := service.NewContactService(contactRepo, partnerRepo, service.WithContactLogger(log))
	activitySvc := service.NewActivityService(
		activityRepo, partnerRepo, contactRepo, service.WithActivityLogger(log))
	opptySvc := service.NewOpportunityService(
		opptyRepo, partnerRepo, currencySvc, service.WithOpportunityLogger(log))
	leadSvc := service.NewLeadService(
		leadRepo, partnerSvc, partnerRepo, service.WithLeadLogger(log))
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
		service.WithSalesOrderLogger(log))
	// 报价单要在转换时建订单，所以它依赖销售订单服务；顺序不能反。
	quotationSvc := service.NewQuotationService(
		quotationRepo, partnerRepo, productRepo, sourceRepo, currencySvc, docNoSvc, orderSvc,
		service.WithQuotationLogger(log))
	// 合同只从报价单或订单抄快照，读的是两边仓储而不是两个服务：
	// 它要的是"那张单现在是什么状态、金额多少"，不是"能不能改"。
	contractSvc := service.NewContractService(
		contractRepo, quotationRepo, orderRepo, currencySvc, docNoSvc,
		service.WithContractLogger(log))

	// 打印是附加能力：缺中文字体或模板写坏只降级，不让服务起不来。
	pdfSvc := buildPDFService(cfg, log, quotationRepo, orderRepo, contractRepo, currencySvc)
	purchaseSvc := service.NewPurchaseOrderService(
		purchaseRepo, partnerRepo, productRepo, sourceRepo, orderRepo, currencySvc, docNoSvc,
		service.WithPurchaseOrderLogger(log))
	// 出货批次管的是"订单行已经被出掉多少"，所以它依赖销售订单仓储（读行与状态），
	// 而不是依赖销售订单服务——服务层之间互相调用会让数量校验绕两圈才算得清。
	shipmentSvc := service.NewShipmentService(
		shipmentRepo, orderRepo, chargeRepo, customsRepo, docNoSvc, service.WithShipmentLogger(log))
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
		paymentRepo, receivableRepo, payableRepo, currencySvc, docNoSvc,
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
	// 报表只读不写，也不缓存：数字要能和明细逐笔对上。
	reportSvc := service.NewReportService(reportRepo, service.WithReportLogger(log))
	dashboardSvc := service.NewDashboardService(productRepo, partnerRepo, c, service.WithDashboardLogger(log))

	// ---- HTTP 层 ----
	engine := server.New(cfg, log, authSvc, server.Handlers{
		Auth:       handler.NewAuthHandler(authSvc),
		Product:    handler.NewProductHandler(productSvc),
		Partner:    handler.NewPartnerHandler(partnerSvc, creditSvc),
		Currency:   handler.NewCurrencyHandler(currencySvc),
		Rate:       handler.NewExchangeRateHandler(rateSvc),
		Contact:    handler.NewContactHandler(contactSvc),
		Activity:   handler.NewActivityHandler(activitySvc),
		Oppty:      handler.NewOpportunityHandler(opptySvc),
		Lead:       handler.NewLeadHandler(leadSvc),
		Source:     handler.NewProductSourceHandler(sourceSvc),
		Quotation:  handler.NewQuotationHandler(quotationSvc),
		Contract:   handler.NewContractHandler(contractSvc),
		PDF:        handler.NewPDFHandler(pdfSvc),
		SalesOrder: handler.NewSalesOrderHandler(orderSvc),
		Purchase:   handler.NewPurchaseOrderHandler(purchaseSvc),
		Shipment:   handler.NewShipmentHandler(shipmentSvc),
		Charge:     handler.NewShipmentChargeHandler(chargeSvc),
		Customs:    handler.NewCustomsDeclarationHandler(customsSvc),
		Receivable: handler.NewReceivableHandler(receivableSvc),
		Payable:    handler.NewPayableHandler(payableSvc),
		Payment:    handler.NewPaymentHandler(paymentSvc),
		Report:     handler.NewReportHandler(reportSvc),
		Stock:      handler.NewStockHandler(stockSvc),
		StockDoc:   handler.NewStockDocumentHandler(stockDocSvc),
		Dashboard:  handler.NewDashboardHandler(dashboardSvc),
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
	currencies service.CurrencyService,
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
	return service.NewPDFService(quotations, orders, contracts, currencies, renderer,
		service.CompanyInfo{
			Name:    cfg.PDF.CompanyName,
			Address: cfg.PDF.CompanyAddress,
			Contact: cfg.PDF.CompanyContact,
		},
		service.WithPDFLogger(log),
	)
}
