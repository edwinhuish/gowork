package bootstrap

import (
	"context"
	"time"

	"gowork_server/internal/model"
	"gowork_server/internal/pkg/money"
	"gowork_server/internal/repository"
	"gowork_server/internal/service"
)

const defaultAdminPassword = "admin123"

// seedDeps 是灌种子要用到的仓储与服务。
//
// 收成一个结构体而不是一长串参数：演示场景从"主数据"扩到"整条业务链"之后，
// 依赖已经有二十来个，逐个位置传参没人读得出哪个是哪个。基础数据只用前四项
// （Auth / Users / Currencies / Rates），其余都是演示链路要的。
type seedDeps struct {
	Auth     service.AuthService
	Users    repository.UserRepository
	Partners repository.PartnerRepository
	Products repository.ProductRepository
	Sources  service.ProductSourceService

	Currencies repository.CurrencyRepository
	Rates      repository.ExchangeRateRepository

	Contacts      service.ContactService
	Activities    service.ActivityService
	Leads         service.LeadService
	Opportunities service.OpportunityService
	Clauses       service.ClauseService

	Quotations service.QuotationService
	Contracts  service.ContractService
	Orders     service.SalesOrderService
	Purchases  service.PurchaseOrderService
	Shipments  service.ShipmentService
	Charges    service.ShipmentChargeService
	Customs    service.CustomsDeclarationService

	Receivables service.ReceivableService
	Payables    service.PayableService
	Payments    service.PaymentService
	StockDocs   service.StockDocumentService
	Receipts    service.PurchaseReceiptService
	Inspections service.InspectionService
	// ProductImages 只在演示数据里用：给每个产品画一张主图。
	ProductImages service.ProductImageService
}

// seed 只在数据为空时写入，重复启动不会产生重复数据。
//
// 分两层，开关只有第二个：
//
//   - 基础数据（无条件）：默认管理员与币种汇率。空库第一次启动要能登进去，
//     也要有本币与汇率才能开单，所以不给开关——它从来不是"演示"的一部分。
//   - 演示数据（database.demo_seed）：示例主数据与整条业务链。
//
// 演示这一段内部分两步：主数据 → 单据。两者都要有汇率才能跑：报价、订单、
// 收付款都要按某天的汇率折本币，缺一条就报"缺少某日汇率"。
func (a *App) seed(ctx context.Context, d seedDeps, withDemo bool) error {
	admin, err := a.seedAdmin(ctx, d.Auth, d.Users)
	if err != nil {
		return err
	}
	if err := a.seedCurrencies(ctx, d.Currencies, d.Rates); err != nil {
		return err
	}
	if !withDemo {
		return nil
	}

	master, err := a.seedDemoMaster(ctx, d, admin)
	if err != nil || master == nil {
		// master 为 nil 表示库里已经有主数据（此前灌过），演示单据也就不再补。
		return err
	}
	return a.seedBusiness(ctx, d, master)
}

// seedCurrencies 写入本币与两种常用外币，并铺一段历史汇率。
//
// 历史汇率不是"多给点数据"：报价与订单会锁当天的汇率，收付款取**到账日**的汇率，
// 两者的差就是汇兑损益。只留今天一条，汇兑损益报表永远是空的。
//
// 这些汇率只是初始占位值（来源标成"系统初始化"）：投产前要按实际行情把自己用的
// 那几种货币的汇率录一遍，否则报价里的本币金额是按假行情算的。
func (a *App) seedCurrencies(
	ctx context.Context,
	currencies repository.CurrencyRepository,
	rates repository.ExchangeRateRepository,
) error {
	n, err := currencies.Count(ctx)
	if err != nil || n > 0 {
		return err
	}

	demoCurrencies := []model.Currency{
		{Code: "CNY", Name: "人民币", Symbol: "¥", DecimalPlaces: 2, Status: model.EnableStatusOn},
		{Code: "USD", Name: "美元", Symbol: "$", DecimalPlaces: 2, Status: model.EnableStatusOn},
		{Code: "EUR", Name: "欧元", Symbol: "€", DecimalPlaces: 2, Status: model.EnableStatusOn},
	}
	for i := range demoCurrencies {
		if err := currencies.Create(ctx, &demoCurrencies[i]); err != nil {
			return err
		}
	}
	if err := currencies.SetBase(ctx, demoCurrencies[0].ID); err != nil {
		return err
	}

	// 今天的汇率必须齐：缺当天汇率时报价与开单都过不去。
	type rateSpec struct {
		code  string
		day   int
		value string
	}
	specs := []rateSpec{
		{"USD", 0, "7.10"},
		{"EUR", 0, "7.75"},
		{"USD", -3, "7.09"},
		{"USD", -8, "7.06"},
		{"USD", -30, "7.12"},
		{"USD", -45, "7.08"},
		{"USD", -60, "7.05"},
		{"USD", -90, "7.02"},
		{"EUR", -45, "7.72"},
		{"EUR", -90, "7.68"},
	}

	demoRates := make([]model.ExchangeRate, 0, len(specs))
	for _, spec := range specs {
		demoRates = append(demoRates, model.ExchangeRate{
			CurrencyCode:  spec.code,
			BaseCode:      "CNY",
			Rate:          money.MustParse(spec.value),
			RateType:      "middle",
			EffectiveDate: model.DateOnly(time.Now().AddDate(0, 0, spec.day)),
			Source:        "系统初始化",
		})
	}
	for i := range demoRates {
		if err := rates.Create(ctx, &demoRates[i]); err != nil {
			return err
		}
	}

	a.Log.Info("已写入币种与汇率", "currencies", len(demoCurrencies), "rates", len(demoRates))
	return nil
}

func (a *App) seedAdmin(
	ctx context.Context,
	auth service.AuthService,
	users repository.UserRepository,
) (*model.User, error) {
	n, err := users.Count(ctx)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		// 已有账号：取回 admin 当演示数据的负责人。
		existing, err := users.FindByUsername(ctx, "admin")
		if err != nil {
			return nil, nil
		}
		return existing, nil
	}

	hash, err := auth.HashPassword(defaultAdminPassword)
	if err != nil {
		return nil, err
	}
	admin := &model.User{
		Username: "admin",
		Password: hash,
		Nickname: "管理员",
		Role:     model.RoleAdmin,
		Status:   model.EnableStatusOn,
	}
	if err := users.Create(ctx, admin); err != nil {
		return nil, err
	}

	a.Log.Warn("已创建默认管理员账号，请尽快修改密码",
		"username", "admin", "password", defaultAdminPassword)
	return admin, nil
}
