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
			&model.SalesOrder{},
			&model.SalesOrderItem{},
			&model.PurchaseOrder{},
			&model.PurchaseOrderItem{},
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
	orderRepo := repository.NewSalesOrderRepository(db)
	purchaseRepo := repository.NewPurchaseOrderRepository(db)

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
	quotationSvc := service.NewQuotationService(
		quotationRepo, partnerRepo, productRepo, sourceRepo, currencySvc, docNoSvc,
		service.WithQuotationLogger(log))
	orderSvc := service.NewSalesOrderService(
		orderRepo, partnerRepo, productRepo, currencySvc, docNoSvc,
		service.WithSalesOrderLogger(log))
	purchaseSvc := service.NewPurchaseOrderService(
		purchaseRepo, partnerRepo, productRepo, sourceRepo, orderRepo, currencySvc, docNoSvc,
		service.WithPurchaseOrderLogger(log))
	dashboardSvc := service.NewDashboardService(productRepo, partnerRepo, c, service.WithDashboardLogger(log))

	// ---- HTTP 层 ----
	engine := server.New(cfg, log, authSvc, server.Handlers{
		Auth:       handler.NewAuthHandler(authSvc),
		Product:    handler.NewProductHandler(productSvc),
		Partner:    handler.NewPartnerHandler(partnerSvc),
		Currency:   handler.NewCurrencyHandler(currencySvc),
		Rate:       handler.NewExchangeRateHandler(rateSvc),
		Contact:    handler.NewContactHandler(contactSvc),
		Activity:   handler.NewActivityHandler(activitySvc),
		Oppty:      handler.NewOpportunityHandler(opptySvc),
		Lead:       handler.NewLeadHandler(leadSvc),
		Source:     handler.NewProductSourceHandler(sourceSvc),
		Quotation:  handler.NewQuotationHandler(quotationSvc),
		SalesOrder: handler.NewSalesOrderHandler(orderSvc),
		Purchase:   handler.NewPurchaseOrderHandler(purchaseSvc),
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
