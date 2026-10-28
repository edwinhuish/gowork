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
	Auth      *handler.AuthHandler
	Product   *handler.ProductHandler
	Party     *handler.PartyHandler
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

		parties := secured.Group("/parties")
		parties.GET("", h.Party.List(nil))
		parties.POST("", h.Party.Create(nil))
		parties.GET("/:id", h.Party.Get(nil))
		parties.PUT("/:id", h.Party.Update(nil))
		parties.DELETE("/:id", h.Party.Delete(nil))

		// 兼容入口。客户与供应商已经合成一张表，但前端页面按 resource 名拼 URL，
		// 且外部可能存着 /customers/12 这类链接。这两组路由把同一份数据
		// 按角色切开暴露，行为与合并前一致。
		// 常量不能取地址，先落成变量再传指针。
		customerRole := model.PartyRoleCustomer
		supplierRole := model.PartyRoleSupplier
		customers := secured.Group("/customers")
		customers.GET("", h.Party.List(&customerRole))
		customers.POST("", h.Party.Create(&customerRole))
		customers.GET("/:id", h.Party.Get(&customerRole))
		customers.PUT("/:id", h.Party.Update(&customerRole))
		customers.DELETE("/:id", h.Party.Delete(&customerRole))

		suppliers := secured.Group("/suppliers")
		suppliers.GET("", h.Party.List(&supplierRole))
		suppliers.POST("", h.Party.Create(&supplierRole))
		suppliers.GET("/:id", h.Party.Get(&supplierRole))
		suppliers.PUT("/:id", h.Party.Update(&supplierRole))
		suppliers.DELETE("/:id", h.Party.Delete(&supplierRole))
	}

	return engine
}

func ginMode(cfg *config.Config) string {
	if cfg.HTTP.Mode == "release" || cfg.IsProd() {
		return gin.ReleaseMode
	}
	return gin.DebugMode
}
