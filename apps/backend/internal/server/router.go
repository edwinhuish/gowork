// Package server 负责组装 gin 引擎与路由表。
//
// 路由集中在这一处定义，方便一眼看清 API 全貌；handler 本身不关心自己被挂在哪个路径下。
package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/handler"
	"github.com/edwinhuish/gowork/internal/server/middleware"
	"github.com/edwinhuish/gowork/internal/service"
)

// Handlers 汇总所有 HTTP handler，由 bootstrap 注入。
type Handlers struct {
	Auth      *handler.AuthHandler
	Product   *handler.ProductHandler
	Supplier  *handler.SupplierHandler
	Customer  *handler.CustomerHandler
	Dashboard *handler.DashboardHandler
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

		suppliers := secured.Group("/suppliers")
		suppliers.GET("", h.Supplier.List)
		suppliers.POST("", h.Supplier.Create)
		suppliers.GET("/:id", h.Supplier.Get)
		suppliers.PUT("/:id", h.Supplier.Update)
		suppliers.DELETE("/:id", h.Supplier.Delete)

		customers := secured.Group("/customers")
		customers.GET("", h.Customer.List)
		customers.POST("", h.Customer.Create)
		customers.GET("/:id", h.Customer.Get)
		customers.PUT("/:id", h.Customer.Update)
		customers.DELETE("/:id", h.Customer.Delete)
	}

	return engine
}

func ginMode(cfg *config.Config) string {
	if cfg.HTTP.Mode == "release" || cfg.IsProd() {
		return gin.ReleaseMode
	}
	return gin.DebugMode
}
