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
	Partner   *handler.PartnerHandler
	Currency  *handler.CurrencyHandler
	Rate      *handler.ExchangeRateHandler
	Contact   *handler.ContactHandler
	Activity  *handler.ActivityHandler
	Oppty     *handler.OpportunityHandler
	Lead      *handler.LeadHandler
	Source    *handler.ProductSourceHandler
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

		// 兼容入口。客户与供应商已经合成一张表，但前端页面按 resource 名拼 URL，
		// 且外部可能存着 /customers/12 这类链接。这两组路由把同一份数据
		// 按角色切开暴露，行为与合并前一致。
		// 常量不能取地址，先落成变量再传指针。
		customerRole := model.PartnerRoleCustomer
		supplierRole := model.PartnerRoleSupplier
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
	}

	return engine
}

func ginMode(cfg *config.Config) string {
	if cfg.HTTP.Mode == "release" || cfg.IsProd() {
		return gin.ReleaseMode
	}
	return gin.DebugMode
}
