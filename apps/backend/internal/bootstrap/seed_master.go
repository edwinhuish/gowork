package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// demoMaster 是演示数据里"长期不变"的那一层：主体、产品，以及它们的 id。
//
// 后面所有单据都按编码取 id，而不是按下标取切片元素——
// 单据之间是交叉引用的（一张出货票同时挂着客户、货代、订单行），
// 用下标写一遍就会在增删一条演示数据时全盘错位。
type demoMaster struct {
	adminID    uint
	ownerName  string
	suppliers  map[string]*model.Partner
	forwarders map[string]*model.Partner
	customers  map[string]*model.Partner
	products   map[string]*model.Product
}

// day 返回相对今天偏移若干天的日期字符串（YYYY-MM-DD）。
//
// 演示数据里的日期必须相对今天算：账龄分桶、回款率、待办都以"今天"为基准，
// 写死具体日期的话，这份数据过一两个月就自己失效了。
func day(offset int) string {
	return time.Now().AddDate(0, 0, offset).Format("2006-01-02")
}

// seedBusiness 在主数据之上灌整条业务演示链：CRM 记录 → 交易单据 → 库存。
//
// 三段是有依赖的：CRM 里的跟进与商机挂在客户上，交易单据要客户与产品，
// 库存要按已出货的数量给足，顺序反过来就会缺这缺那。
func (a *App) seedBusiness(ctx context.Context, d seedDeps, master *demoMaster) error {
	// 产品图先灌：卡片视图靠它，而后面的单据与图片没有依赖。
	if err := a.seedProductImages(ctx, d, master); err != nil {
		return err
	}
	if err := a.seedCRM(ctx, d, master); err != nil {
		return err
	}
	if err := a.seedTrading(ctx, d, master); err != nil {
		return err
	}
	return a.seedStockAndFinance(ctx, d, master)
}

// seedMasterData 写入往来单位、产品与条款模板。
//
// 演示数据刻意包含几种"合并前无法表达"的记录：客供两用（role=3）、
// 个人主体（entity_type=1）、以及不同分级 / 账期 / 信用额度的客户，
// 这样列表、筛选、概览计数与额度拦截一跑起来就能看出模型是否真的生效。
func (a *App) seedMasterData(
	ctx context.Context,
	d seedDeps,
	admin *model.User,
) (*demoMaster, error) {
	n, err := d.Partners.Count(ctx)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, nil
	}

	ownerID := uint(0)
	ownerName := ""
	if admin != nil {
		ownerID = admin.ID
		ownerName = admin.Nickname
	}

	demoPartners := []model.Partner{
		// ---- 供应商 ----
		{
			Code: "SUP001", Name: "永康市华信五金厂",
			Role: model.PartnerRoleSupplier, EntityType: model.PartnerEntityCompany,
			Country: "China", Region: "华东", ContactName: "陈先生", Phone: "13800000001",
			Email: "chen@huaxin-hardware.cn", Address: "浙江省金华市永康市芝英镇",
			TradeTerm: model.TradeTermFOB, SettlementMethod: model.SettlementTT,
			CreditDays: 30, OwnerID: ownerID,
		},
		{
			Code: "SUP002", Name: "揭阳铭泰不锈钢制品",
			Role: model.PartnerRoleSupplier, EntityType: model.PartnerEntityCompany,
			Country: "China", Region: "华南", ContactName: "林小姐", Phone: "13800000002",
			Email: "lin@mingtai-ss.com", Address: "广东省揭阳市榕城区",
			TradeTerm: model.TradeTermEXW, SettlementMethod: model.SettlementTT,
			CreditDays: 15, OwnerID: ownerID,
		},
		{
			Code: "SUP003", Name: "宁波北仑精密压铸有限公司",
			Role: model.PartnerRoleSupplier, EntityType: model.PartnerEntityCompany,
			Country: "China", Region: "华东", ContactName: "吴工", Phone: "13800000004",
			Address: "浙江省宁波市北仑区", TradeTerm: model.TradeTermFOB,
			SettlementMethod: model.SettlementTT, CreditDays: 30, OwnerID: ownerID,
		},
		{
			Code: "SUP004", Name: "佛山南海包装材料厂",
			Role: model.PartnerRoleSupplier, EntityType: model.PartnerEntityCompany,
			Country: "China", Region: "华南", ContactName: "梁经理", Phone: "13800000005",
			Address: "广东省佛山市南海区", TradeTerm: model.TradeTermEXW,
			SettlementMethod: model.SettlementTT, CreditDays: 7, OwnerID: ownerID,
		},
		{
			Code: "SUP005", Name: "青岛恒达索具制造",
			Role: model.PartnerRoleSupplier, EntityType: model.PartnerEntityCompany,
			Country: "China", Region: "华北", ContactName: "孙经理", Phone: "13800000006",
			Address: "山东省青岛市黄岛区", TradeTerm: model.TradeTermFOB,
			SettlementMethod: model.SettlementTT, CreditDays: 30,
		},

		// ---- 货代 ----
		{
			Code: "FWD001", Name: "上海联航国际货运代理",
			Role: model.PartnerRoleForwarder, EntityType: model.PartnerEntityCompany,
			Country: "China", Region: "华东", ContactName: "赵小姐", Phone: "021-60000001",
			Email: "ops@lianhang-logistics.cn", Address: "上海市虹口区东大名路",
			SettlementMethod: model.SettlementTT, CreditDays: 15, OwnerID: ownerID,
			Remark: "美线与南美线，惯走上海港",
		},
		{
			Code: "FWD002", Name: "宁波中远物流",
			Role: model.PartnerRoleForwarder, EntityType: model.PartnerEntityCompany,
			Country: "China", Region: "华东", ContactName: "周先生", Phone: "0574-80000002",
			Address: "浙江省宁波市北仑区", SettlementMethod: model.SettlementTT,
			CreditDays: 30, Remark: "欧线为主，宁波港出货",
		},

		// ---- 客户 ----
		{
			Code: "CUS001", Name: "Ferreteria El Sol",
			Role: model.PartnerRoleCustomer, EntityType: model.PartnerEntityCompany,
			Country: "Ecuador", Region: "南美", CustomerGroup: "五金零售连锁",
			Grade: model.PartnerGradeA, ContactName: "Carlos Mendoza",
			Phone: "+593 9 0000 0001", Email: "carlos@elsol.com.ec",
			Address: "Av. 9 de Octubre, Guayaquil", TradeTerm: model.TradeTermCIF,
			SettlementMethod: model.SettlementTT, CreditDays: 30,
			CurrencyCode: "USD", OwnerID: ownerID,
			Remark: "年采购约 40 万美元，价格敏感但复购稳定",
		},
		{
			Code: "BOTH001", Name: "宁波华翔工贸",
			Role:       model.PartnerRoleCustomer | model.PartnerRoleSupplier,
			EntityType: model.PartnerEntityCompany,
			Country:    "China", Region: "华东", CustomerGroup: "贸易商",
			Grade: model.PartnerGradeB, ContactName: "周经理", Phone: "13800000003",
			Address: "浙江省宁波市鄞州区", TradeTerm: model.TradeTermFOB,
			SettlementMethod: model.SettlementTT, CreditDays: 45,
			CurrencyCode: "USD", OwnerID: ownerID,
			Remark: "既买成品，又把包装物料卖回给我们 —— 客供两用",
		},
		{
			Code: "PER001", Name: "王海（个人买手）",
			Role: model.PartnerRoleCustomer, EntityType: model.PartnerEntityPerson,
			Country: "Ecuador", Region: "南美", CustomerGroup: "个人买手",
			Grade: model.PartnerGradeC, ContactName: "王海", Phone: "+593 9 0000 0002",
			Address: "Guayaquil", TradeTerm: model.TradeTermFOB,
			SettlementMethod: model.SettlementTT, CreditDays: 0,
			CurrencyCode: "USD", OwnerID: ownerID,
			Remark: "个人收款的买手，报关资料用个人身份",
		},
		{
			Code: "CUS004", Name: "Ferreteria Central S.A.",
			Role: model.PartnerRoleCustomer, EntityType: model.PartnerEntityCompany,
			Country: "Panama", Region: "中美", CustomerGroup: "五金零售连锁",
			Grade: model.PartnerGradeB, ContactName: "Ana Torres",
			Phone: "+507 6000 0001", Email: "ana@central.com.pa",
			Address: "Ciudad de Panamá", TradeTerm: model.TradeTermFOB,
			SettlementMethod: model.SettlementDP, CreditDays: 45,
			CurrencyCode: "USD", OwnerID: ownerID,
		},
		{
			Code: "CUS005", Name: "Home Depot Mexico S.A. de C.V.",
			Role: model.PartnerRoleCustomer, EntityType: model.PartnerEntityCompany,
			Country: "Mexico", Region: "北美", CustomerGroup: "建材大卖场",
			Grade: model.PartnerGradeA, ContactName: "Luis Ramírez",
			Phone: "+52 55 0000 0001", Email: "luis@hdmexico.com",
			Address: "Av. Insurgentes Sur, CDMX", TradeTerm: model.TradeTermFOB,
			SettlementMethod: model.SettlementLC, CreditDays: 60,
			CreditLimit: money.MustParse("500000"), CurrencyCode: "USD",
			OwnerID: ownerID,
			Remark:  "有信用额度，订单确认时会校验；超额需管理员放行",
		},
		{
			Code: "CUS006", Name: "Construcciones Andinas S.A.C.",
			Role: model.PartnerRoleCustomer, EntityType: model.PartnerEntityCompany,
			Country: "Peru", Region: "南美", CustomerGroup: "工程承包商",
			Grade: model.PartnerGradeC, ContactName: "Rosa Quispe",
			Phone: "+51 1 000 0001", Address: "Lima", TradeTerm: model.TradeTermCFR,
			SettlementMethod: model.SettlementOA, CreditDays: 60,
			CurrencyCode: "USD",
			Remark:       "账期太长，接单前先复核额度",
		},
	}
	for i := range demoPartners {
		if err := d.Partners.Create(ctx, &demoPartners[i]); err != nil {
			return nil, fmt.Errorf("写入往来单位 %s: %w", demoPartners[i].Code, err)
		}
	}

	master := &demoMaster{
		adminID:    ownerID,
		ownerName:  ownerName,
		suppliers:  map[string]*model.Partner{},
		forwarders: map[string]*model.Partner{},
		customers:  map[string]*model.Partner{},
		products:   map[string]*model.Product{},
	}
	for i := range demoPartners {
		p := &demoPartners[i]
		if p.Role&model.PartnerRoleForwarder != 0 {
			master.forwarders[p.Code] = p
		}
		if p.Role&model.PartnerRoleSupplier != 0 {
			master.suppliers[p.Code] = p
		}
		if p.Role&model.PartnerRoleCustomer != 0 {
			master.customers[p.Code] = p
		}
	}

	if err := a.seedProducts(ctx, d, master); err != nil {
		return nil, err
	}
	if err := a.seedClauses(ctx, d); err != nil {
		return nil, err
	}

	a.Log.Info("已写入演示主数据",
		"partners", len(demoPartners), "products", len(master.products))
	return master, nil
}

// seedProducts 写入 16 个产品，覆盖 8 个类别。
//
// 每个产品都带箱规与毛净重：出货单的装箱单、订舱需要的 CBM 都由这些字段推出来，
// 全为空时装箱单会是一张只有品名的空壳。
func (a *App) seedProducts(ctx context.Context, d seedDeps, master *demoMaster) error {
	supplier := func(code string) *uint {
		if p, ok := master.suppliers[code]; ok {
			return &p.ID
		}
		return nil
	}

	type spec struct {
		sku, nameCN, nameEN, category, specText, unit string
		supplier                                      string
		cost, sale                                    string
		packQty                                       int
		gross, net                                    string
		cartonSize, hsCode, declaredCN, declaredEN    string
	}
	// 售价按"人民币成本 / 7.1 × 1.2~1.3"折算成美元：成本与本币同口径，
	// 售价是外币报价，两者混着写会让毛利变成 80%+ 这种一眼假的数。
	specs := []spec{
		{"BH01-0001-SS", "304 不锈钢合页 4 寸", "SS Hinge 4\"", "BH", "100×75×3 mm", "PCS", "SUP001", "6.8", "1.20", 100, "18.5", "17.8", "42×28×22CM", "83023000", "不锈钢合页", "Stainless Steel Hinge"},
		{"BH01-0002-CP", "铁合页 4 寸 镀铬", "Iron Hinge 4\" CP", "BH", "100×75×2.5 mm", "PCS", "SUP001", "3.2", "0.56", 200, "21", "20.2", "45×30×24CM", "83023000", "铁合页", "Iron Hinge"},
		{"BH02-0001-BK", "门吸 不锈钢 黑色", "Door Stopper BK", "BH", "Ø45 mm", "PCS", "SUP001", "2.4", "0.42", 300, "14", "13.2", "40×26×20CM", "83024100", "门吸", "Door Stopper"},
		{"BH03-0001-SS", "不锈钢拉手 128mm", "SS Pull Handle 128mm", "BH", "128 mm 孔距", "PCS", "SUP002", "4.6", "0.82", 150, "16.5", "15.8", "44×30×22CM", "83024100", "拉手", "Pull Handle"},
		{"LK01-0001-BK", "球形门锁 黑色", "Round Knob Lock BK", "LK", "60 mm", "SET", "SUP001", "12.5", "2.20", 60, "19", "18.2", "38×26×24CM", "83013000", "球形门锁", "Knob Lock"},
		{"LK02-0001-SS", "执手锁 不锈钢", "Lever Handle Lock SS", "LK", "70 mm 孔距", "SET", "SUP002", "22", "3.90", 40, "22", "21", "40×28×26CM", "83013000", "执手锁", "Lever Lock"},
		{"LK03-0001", "挂锁 50mm 铜芯", "Padlock 50mm", "LK", "50 mm", "PCS", "SUP003", "6.5", "1.15", 120, "17", "16.4", "36×24×20CM", "83011000", "挂锁", "Padlock"},
		{"FS01-0001", "自攻螺丝 4×30 十字", "Self Tapping Screw 4×30", "FS", "4×30 mm / 1000pcs", "BOX", "SUP001", "8", "1.42", 40, "20", "19.4", "40×30×25CM", "73181400", "自攻螺丝", "Self Tapping Screw"},
		{"FS02-0001", "膨胀螺栓 M8", "Expansion Bolt M8", "FS", "M8×80 mm / 100pcs", "BOX", "SUP003", "5.2", "0.92", 60, "18", "17.5", "38×26×22CM", "73181500", "膨胀螺栓", "Expansion Bolt"},
		{"FS03-0001", "六角螺栓 M10×60", "Hex Bolt M10×60", "FS", "M10×60 mm / 100pcs", "BOX", "SUP005", "7.4", "1.31", 50, "19.5", "18.8", "40×28×22CM", "73181500", "六角螺栓", "Hex Bolt"},
		{"HT01-0001", "羊角锤 8oz 木柄", "Claw Hammer 8oz", "HT", "8 oz", "PCS", "SUP003", "9.5", "1.68", 60, "21", "20.2", "42×30×24CM", "82052000", "羊角锤", "Claw Hammer"},
		{"HT02-0001", "活动扳手 10 寸", "Adjustable Wrench 10\"", "HT", "10 inch", "PCS", "SUP003", "13", "2.30", 40, "23", "22.2", "44×30×26CM", "82041200", "活动扳手", "Adjustable Wrench"},
		{"PT01-0001", "冲击钻 13mm 750W", "Impact Drill 13mm", "PT", "750 W / 13 mm", "SET", "SUP003", "88", "15.60", 8, "26", "24.5", "52×36×30CM", "84672100", "冲击钻", "Impact Drill"},
		{"PL01-0001", "不锈钢水龙头 单冷", "SS Faucet Cold", "PL", "G1/2 接口", "PCS", "SUP002", "12", "2.12", 48, "20", "19.2", "42×30×26CM", "84818090", "水龙头", "Faucet"},
		{"MS01-0001", "钢卷尺 5m", "Steel Tape 5m", "MS", "5 m × 19 mm", "PCS", "SUP003", "3.6", "0.64", 120, "16", "15.5", "44×28×20CM", "90178000", "钢卷尺", "Steel Tape"},
		{"PK01-0001", "五层瓦楞纸箱 中号", "Corrugated Carton M", "PK", "五层 / 500×350×300", "PCS", "SUP004", "1.8", "0", 100, "12", "11.6", "80×60×20CM", "48191000", "纸箱", "Carton"},
	}

	for _, s := range specs {
		product := &model.Product{
			SKUCode:           s.sku,
			NameCN:            s.nameCN,
			NameEN:            s.nameEN,
			Category:          s.category,
			Spec:              s.specText,
			Unit:              s.unit,
			SupplierID:        supplier(s.supplier),
			CostPrice:         money.MustParse(s.cost),
			SalePrice:         money.MustParse(s.sale),
			PackQty:           s.packQty,
			CartonGrossWeight: money.MustParse(s.gross),
			CartonNetWeight:   money.MustParse(s.net),
			CartonSize:        s.cartonSize,
			HSCode:            s.hsCode,
			DeclaredNameCN:    s.declaredCN,
			DeclaredNameEN:    s.declaredEN,
			OriginCountry:     "China",
			LegalUnit:         s.unit,
		}
		if err := d.Products.Create(ctx, product); err != nil {
			return fmt.Errorf("写入产品 %s: %w", s.sku, err)
		}
		master.products[s.sku] = product

		// 货源是报价的前置条件：报价要先按货源算成本快照，
		// 没有货源的产品连报价单都开不出来。
		if product.SupplierID == nil {
			continue
		}
		source := dto.ProductSourceInput{
			ProductID:     product.ID,
			PartnerID:     *product.SupplierID,
			SupplierSKU:   "SN-" + s.sku,
			PurchasePrice: dec(s.cost),
			MOQ:           500,
			DeliveryDays:  25,
			Remark:        "演示货源",
		}
		if _, err := d.Sources.Create(ctx, source); err != nil {
			return fmt.Errorf("写入货源 %s: %w", s.sku, err)
		}
	}
	return nil
}

// seedClauses 写入几条常用的合同条款模板。
//
// 模板是主数据、合同上存的是快照：改模板不影响已生成的合同。
// 演示数据里给三条覆盖不同类别（付款 / 质量 / 包装），合同生成时才能真的挑出来。
func (a *App) seedClauses(ctx context.Context, d seedDeps) error {
	templates := []struct {
		code, title, category, body string
	}{
		{
			code: "PAY-TT30", title: "付款方式：30% 订金 + 70% 见提单副本",
			category: "付款",
			body:     "买方应于合同签订后 7 个工作日内支付合同总额 30% 作为订金；余款 70% 于卖方提交提单副本后 7 个工作日内以电汇支付。",
		},
		{
			code: "QLTY-INSP", title: "质量与验货",
			category: "质量",
			body:     "货物须符合本合同附件所列规格。买方有权于装运前在卖方工厂进行验货，验货不合格的批次由卖方负责返工，交期相应顺延。",
		},
		{
			code: "PACK-EXPORT", title: "出口包装要求",
			category: "包装",
			body:     "货物按出口标准包装，外箱须印唛头、箱号与毛净重，单箱毛重不超过 25KG，并适合多次搬运与海运。",
		},
		{
			code: "CLAIM-30D", title: "索赔期限",
			category: "索赔",
			body:     "买方对货物数量或质量的异议应于到港后 30 日内以书面形式提出，逾期视为验收合格。",
		},
	}
	for _, t := range templates {
		in := dto.ClauseInput{
			Code:     t.code,
			Title:    t.title,
			Category: t.category,
			Body:     t.body,
		}
		if _, err := d.Clauses.Create(ctx, in); err != nil {
			return fmt.Errorf("写入条款模板 %s: %w", t.code, err)
		}
	}
	return nil
}
