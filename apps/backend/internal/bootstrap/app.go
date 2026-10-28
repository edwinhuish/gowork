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

	if cfg.Database.AutoMigrate {
		if err := database.AutoMigrate(db,
			&model.User{},
			&model.Supplier{},
			&model.Customer{},
			&model.Product{},
		); err != nil {
			_ = database.Close(db)
			return nil, err
		}
		log.Info("数据库结构已同步")
	}

	c, err := cache.New(cfg.Cache, log)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("初始化缓存: %w", err)
	}

	// ---- 数据访问层 ----
	userRepo := repository.NewUserRepository(db)
	supplierRepo := repository.NewSupplierRepository(db)
	customerRepo := repository.NewCustomerRepository(db)
	productRepo := repository.NewProductRepository(db)

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
	productSvc := service.NewProductService(productRepo, service.WithProductLogger(log))
	supplierSvc := service.NewSupplierService(supplierRepo, service.WithSupplierLogger(log))
	customerSvc := service.NewCustomerService(customerRepo, service.WithCustomerLogger(log))
	dashboardSvc := service.NewDashboardService(productRepo, supplierRepo, customerRepo, c, service.WithDashboardLogger(log))

	// ---- HTTP 层 ----
	engine := server.New(cfg, log, authSvc, server.Handlers{
		Auth:      handler.NewAuthHandler(authSvc),
		Product:   handler.NewProductHandler(productSvc),
		Supplier:  handler.NewSupplierHandler(supplierSvc),
		Customer:  handler.NewCustomerHandler(customerSvc),
		Dashboard: handler.NewDashboardHandler(dashboardSvc),
	})

	app := &App{Config: cfg, Engine: engine, DB: db, Cache: c, Log: log}

	if cfg.Database.Seed {
		if err := app.seed(context.Background(), authSvc, userRepo, supplierRepo, customerRepo, productRepo); err != nil {
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
