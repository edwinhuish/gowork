package bootstrap

import (
	"context"
	"fmt"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/money"
)

// dec 是演示数据里写金额的简写：常量金额用字符串写，避免浮点字面量在定点数上丢精度。
func dec(s string) money.Decimal { return money.MustParse(s) }

// seedTrading 走完一条真实的外销主链：
//
//	报价单 → 转销售订单 → 确认 → 出货票（装箱 / 报关 / 运杂费 / 物流节点）
//	→ 应收 → 收款核销
//
// 另外造两张分叉：一张只确认不出货的订单、一张草稿订单与草稿出货票，
// 这样列表里能看到不同状态，状态机与「还能出多少」这类判断才有对照物。
func (a *App) seedTrading(ctx context.Context, d seedDeps, m *demoMaster) error {
	sun := m.customers["CUS001"]
	hd := m.customers["CUS005"]
	huaxiang := m.customers["BOTH001"]
	central := m.customers["CUS004"]
	if sun == nil || hd == nil || huaxiang == nil || central == nil {
		return fmt.Errorf("演示数据缺少客户档案")
	}

	// ---- 报价单：一张已转订单、一张已确认待转、一张草稿 ----
	q1, err := d.Quotations.Create(ctx, dto.QuotationInput{
		PartnerID:    sun.ID,
		TradeTerm:    string(model.TradeTermCIF),
		CurrencyCode: "USD",
		ValidUntil:   day(25),
		Remark:       "Q1 复购报价，含门锁系列",
		Items: []dto.QuotationItemInput{
			{ProductID: m.products["BH01-0001-SS"].ID, Qty: dec("6000"), UnitPrice: dec("1.25")},
			{ProductID: m.products["LK01-0001-BK"].ID, Qty: dec("2400"), UnitPrice: dec("2.30")},
			{ProductID: m.products["BH03-0001-SS"].ID, Qty: dec("3000"), UnitPrice: dec("0.85")},
		},
	})
	if err != nil {
		return fmt.Errorf("写入报价单（已转订单）: %w", err)
	}
	if _, err := d.Quotations.UpdateStatus(ctx, q1.ID, dto.QuotationStatusInput{Status: int8(model.DocumentStatusConfirmed)}); err != nil {
		return fmt.Errorf("确认报价单: %w", err)
	}

	q2, err := d.Quotations.Create(ctx, dto.QuotationInput{
		PartnerID:    hd.ID,
		TradeTerm:    string(model.TradeTermFOB),
		CurrencyCode: "USD",
		ValidUntil:   day(14),
		Remark:       "墨西哥 Q1 补货报价，待客户走内部审批",
		Items: []dto.QuotationItemInput{
			{ProductID: m.products["FS01-0001"].ID, Qty: dec("8000"), UnitPrice: dec("1.45")},
			{ProductID: m.products["HT01-0001"].ID, Qty: dec("2000"), UnitPrice: dec("1.72")},
			{ProductID: m.products["PT01-0001"].ID, Qty: dec("300"), UnitPrice: dec("15.80")},
		},
	})
	if err != nil {
		return fmt.Errorf("写入报价单（已确认待转）: %w", err)
	}
	if _, err := d.Quotations.UpdateStatus(ctx, q2.ID, dto.QuotationStatusInput{Status: int8(model.DocumentStatusConfirmed)}); err != nil {
		return fmt.Errorf("确认报价单: %w", err)
	}

	if _, err := d.Quotations.Create(ctx, dto.QuotationInput{
		PartnerID:    central.ID,
		TradeTerm:    string(model.TradeTermFOB),
		CurrencyCode: "USD",
		ValidUntil:   day(20),
		Remark:       "草稿：等客户确认数量再发",
		Items: []dto.QuotationItemInput{
			{ProductID: m.products["LK02-0001-SS"].ID, Qty: dec("1200"), UnitPrice: dec("4.05")},
		},
	}); err != nil {
		return fmt.Errorf("写入报价单（草稿）: %w", err)
	}

	// ---- 合同：从报价单生成，两期付款，挂两条条款快照 ----
	clauseIDs := a.firstClauseIDs(ctx, d, 2)
	if _, err := d.Contracts.Generate(ctx, dto.ContractGenerateInput{
		Type:       int8(model.ContractTypeSales),
		SourceType: int8(model.ContractSourceQuotation),
		SourceID:   q1.ID,
		SignDate:   day(-5),
		// 分期合计必须等于合同总额，否则服务端拒绝（合计不等时对账单永远差一截）。
		// 15570 = 6000×1.25 + 2400×2.30 + 3000×0.85，与上面那张报价单一致。
		Terms: []dto.ContractTermInput{
			{Name: "订金 30%", Amount: dec("4671"), DueDate: day(10), Trigger: "合同签订后 7 个工作日"},
			{Name: "尾款 70%", Amount: dec("10899"), DueDate: day(40), Trigger: "见提单副本后 7 个工作日"},
		},
		ClauseIDs: clauseIDs,
	}); err != nil {
		// 合同不是主链的必经环节（报价也能直接转订单），失败只记日志，不挡住后面的单据。
		a.Log.Warn("演示合同生成失败，跳过", "err", err)
	}

	// ---- 销售订单 ----
	o1Detail, err := d.Quotations.Convert(ctx, q1.ID, dto.QuotationConvertInput{DeliveryDate: day(15)})
	if err != nil {
		return fmt.Errorf("报价转订单: %w", err)
	}
	if o1Detail.SalesOrderID == nil {
		return fmt.Errorf("报价转订单没有返回订单 id")
	}
	orderID := *o1Detail.SalesOrderID
	o1, err := d.Orders.UpdateStatus(ctx, orderID, dto.SalesOrderStatusInput{Status: int8(model.DocumentStatusConfirmed)})
	if err != nil {
		return fmt.Errorf("确认销售订单: %w", err)
	}
	if o1.Status == model.DocumentStatusConfirmed {
		if _, err := d.Orders.UpdateStatus(ctx, orderID, dto.SalesOrderStatusInput{Status: int8(model.DocumentStatusPreparing)}); err != nil {
			return fmt.Errorf("订单转备货中: %w", err)
		}
	}
	o1, err = d.Orders.Get(ctx, orderID)
	if err != nil {
		return fmt.Errorf("读取销售订单: %w", err)
	}

	o3, err := a.createOrder(ctx, d, huaxiang.ID, []dto.SalesOrderItemInput{
		{ProductID: m.products["FS02-0001"].ID, Qty: dec("3000"), UnitPrice: dec("1.05")},
		{ProductID: m.products["FS03-0001"].ID, Qty: dec("2000"), UnitPrice: dec("1.35")},
	}, "USD", day(-40), day(-10))
	if err != nil {
		return err
	}
	if _, err := d.Orders.UpdateStatus(ctx, o3.ID, dto.SalesOrderStatusInput{Status: int8(model.DocumentStatusConfirmed)}); err != nil {
		return fmt.Errorf("确认销售订单（华翔）: %w", err)
	}

	o4, err := a.createOrder(ctx, d, central.ID, []dto.SalesOrderItemInput{
		{ProductID: m.products["LK03-0001"].ID, Qty: dec("3600"), UnitPrice: dec("1.30")},
		{ProductID: m.products["MS01-0001"].ID, Qty: dec("2400"), UnitPrice: dec("0.75")},
	}, "USD", day(-3), day(35))
	if err != nil {
		return err
	}
	if _, err := d.Orders.UpdateStatus(ctx, o4.ID, dto.SalesOrderStatusInput{Status: int8(model.DocumentStatusConfirmed)}); err != nil {
		return fmt.Errorf("确认销售订单（Central）: %w", err)
	}

	// 草稿订单：只到草稿就停，列表里用来区分"还没确认"与"已确认"。
	if _, err := a.createOrder(ctx, d, hd.ID, []dto.SalesOrderItemInput{
		{ProductID: m.products["BH02-0001-BK"].ID, Qty: dec("5000"), UnitPrice: dec("0.50")},
	}, "USD", day(0), day(45)); err != nil {
		return err
	}

	// ---- 出货票：一票已出货（带装箱 / 报关 / 运杂费 / 物流），一票草稿 ----
	shipmentID, err := a.seedShipment(ctx, d, m, o1, shipPlan{
		shipDate:    day(-20),
		forwarderID: m.forwarders["FWD001"].ID,
		logistics:   true,
		charges:     true,
		customs:     true,
		confirm:     true,
	})
	if err != nil {
		return err
	}

	if _, err := a.seedShipment(ctx, d, m, o4, shipPlan{
		shipDate:    day(-1),
		forwarderID: m.forwarders["FWD002"].ID,
		partial:     true,
	}); err != nil {
		return err
	}

	// ---- 应收与收款 ----
	r1, err := d.Receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   o1.ID,
		Amount:     dec("0"),
		DueDate:    day(-30),
		Remark:     "整单开票，账期 30 天",
	})
	if err != nil {
		return fmt.Errorf("写入应收（已转订单那张）: %w", err)
	}
	r2, err := d.Receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   o3.ID,
		Amount:     dec("0"),
		DueDate:    day(-12),
		Remark:     "华翔账期 45 天，已到期",
	})
	if err != nil {
		return fmt.Errorf("写入应收（华翔）: %w", err)
	}
	if _, err := d.Receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceSalesOrder),
		SourceID:   o4.ID,
		Amount:     dec("0"),
		DueDate:    day(20),
		Remark:     "Central 账期 45 天，尚未到期",
	}); err != nil {
		return fmt.Errorf("写入应收（Central）: %w", err)
	}
	// 出货票也开一张应收：分票开票是常态，票上挂着它才能在出货详情里看到开票情况。
	if _, err := d.Receivables.Create(ctx, dto.ReceivableInput{
		SourceType: int8(model.SettlementSourceShipment),
		SourceID:   shipmentID,
		Amount:     dec("3000"),
		DueDate:    day(-8),
		Remark:     "按一票出货开票 3000 美元",
	}); err != nil {
		a.Log.Warn("演示应收（按出货）写入失败，跳过", "err", err)
	}

	// 收款：一笔整额核销华翔那张（到账汇率 7.06 低于开单 7.10，产生汇兑收益）；
	// 另一笔部分核销 El Sol，留下未结清余额给账龄与催款清单。
	if _, err := d.Payments.Create(ctx, dto.PaymentInput{
		Direction:    10,
		PartnerID:    huaxiang.ID,
		PayDate:      day(-8),
		CurrencyCode: "USD",
		Amount:       r2.Amount,
		Method:       string(model.SettlementTT),
		Reference:    "TT2601-00871",
		Remark:       "华翔全额付款",
		Allocations: []dto.PaymentAllocationInput{
			{TargetType: 10, TargetID: r2.ID, Amount: r2.Amount},
		},
	}); err != nil {
		return fmt.Errorf("写入收款（华翔）: %w", err)
	}

	partial := money.MustParse("12000")
	if partial.Cmp(r1.Amount) > 0 {
		partial = r1.Amount
	}
	if _, err := d.Payments.Create(ctx, dto.PaymentInput{
		Direction:    10,
		PartnerID:    sun.ID,
		PayDate:      day(-3),
		CurrencyCode: "USD",
		Amount:       partial,
		Method:       string(model.SettlementTT),
		Reference:    "TT2601-01002",
		Remark:       "El Sol 支付部分货款，余额待催",
		Allocations: []dto.PaymentAllocationInput{
			{TargetType: 10, TargetID: r1.ID, Amount: partial},
		},
	}); err != nil {
		return fmt.Errorf("写入收款（El Sol）: %w", err)
	}

	// ---- 采购：以销定采 + 应付 + 付款 ----
	if err := a.seedPurchase(ctx, d, m, o1); err != nil {
		return err
	}

	a.Log.Info("已写入演示业务单据",
		"quotations", 3, "orders", 4, "shipments", 2, "receivables", 4)
	return nil
}

// createOrder 建一张销售订单（草稿）。items 里只写产品与数量，单价由调用方给。
func (a *App) createOrder(
	ctx context.Context,
	d seedDeps,
	partnerID uint,
	items []dto.SalesOrderItemInput,
	currency, orderDate, deliveryDate string,
) (*model.SalesOrder, error) {
	order, err := d.Orders.Create(ctx, dto.SalesOrderInput{
		PartnerID:    partnerID,
		TradeTerm:    string(model.TradeTermFOB),
		CurrencyCode: currency,
		OrderDate:    orderDate,
		DeliveryDate: deliveryDate,
		Items:        items,
	})
	if err != nil {
		return nil, fmt.Errorf("写入销售订单: %w", err)
	}
	return order, nil
}

// shipPlan 描述一张演示出货票要带哪几块数据。
type shipPlan struct {
	shipDate    string
	forwarderID uint
	/** partial 为真时只出第一行的一半，用来演示"还能再出"的剩余数量。 */
	partial   bool
	logistics bool
	charges   bool
	customs   bool
	confirm   bool
}

// seedShipment 按订单行建一张出货票，并按 shipPlan 补齐装箱、物流、费用与报关。
func (a *App) seedShipment(
	ctx context.Context,
	d seedDeps,
	m *demoMaster,
	order *model.SalesOrder,
	plan shipPlan,
) (uint, error) {
	items, err := d.Orders.Get(ctx, order.ID)
	if err != nil {
		return 0, fmt.Errorf("读取订单明细: %w", err)
	}
	if len(items.Items) == 0 {
		return 0, fmt.Errorf("订单 %s 没有明细", order.DocNo)
	}

	allocations := make([]dto.ShipmentAllocationInput, 0, len(items.Items))
	for i, item := range items.Items {
		qty := item.Qty
		if plan.partial && i > 0 {
			continue
		}
		if plan.partial {
			qty = qty.Div(dec("2")).Round(2)
		}
		product := m.productByID(item.ProductID)
		packQty := 100
		if product != nil && product.PackQty > 0 {
			packQty = product.PackQty
		}
		qtyNum := int(qty.Float64())
		cartons := (qtyNum + packQty - 1) / packQty
		if cartons == 0 {
			cartons = 1
		}
		gross := dec("0")
		net := dec("0")
		measurement := ""
		if product != nil && product.CartonGrossWeight.IsPositive() {
			gross = product.CartonGrossWeight.MulInt(int64(cartons))
			net = product.CartonNetWeight.MulInt(int64(cartons))
			measurement = product.CartonSize
		}

		allocations = append(allocations, dto.ShipmentAllocationInput{
			SalesOrderItemID: item.ID,
			Qty:              qty,
			Cartons:          cartons,
			GrossWeight:      gross,
			NetWeight:        net,
			Measurement:      measurement,
		})
	}

	shipment, err := d.Shipments.Create(ctx, dto.ShipmentInput{
		ShipDate:    plan.shipDate,
		ForwarderID: &plan.forwarderID,
		Remark:      "演示出货票",
		Allocations: allocations,
	})
	if err != nil {
		return 0, fmt.Errorf("写入出货票: %w", err)
	}

	if !plan.confirm {
		return shipment.ID, nil
	}

	// 状态机：草稿 → 已确认 → 备货中 → 已出货（到这一步才生成销售出库流水）。
	for _, status := range []model.DocumentStatus{
		model.DocumentStatusConfirmed,
		model.DocumentStatusPreparing,
		model.DocumentStatusShipped,
	} {
		if _, err := d.Shipments.UpdateStatus(ctx, shipment.ID, dto.ShipmentStatusInput{Status: int8(status)}); err != nil {
			return 0, fmt.Errorf("出货票转状态 %d: %w", status, err)
		}
	}

	if plan.logistics {
		if _, err := d.Shipments.UpdateLogistics(ctx, shipment.ID, dto.ShipmentLogisticsInput{
			BLNo:   "COSU6300123456",
			Vessel: "COSCO SHIPPING ROSE / 048E",
			ETD:    plan.shipDate,
			ETA:    day(12),
		}); err != nil {
			return 0, fmt.Errorf("写入物流节点: %w", err)
		}
	}

	if plan.charges {
		freight := m.forwarders["FWD001"]
		charges := []dto.ShipmentChargeInput{
			{ShipmentID: shipment.ID, Type: int8(model.ShipmentChargeFreight), PartnerID: &freight.ID, Amount: dec("820"), CurrencyCode: "USD", Remark: "海运费 20GP"},
			{ShipmentID: shipment.ID, Type: int8(model.ShipmentChargeCustoms), PartnerID: &freight.ID, Amount: dec("1500"), CurrencyCode: "CNY", Remark: "报关与单证费"},
			{ShipmentID: shipment.ID, Type: int8(model.ShipmentChargeTrucking), PartnerID: &freight.ID, Amount: dec("900"), CurrencyCode: "CNY", Remark: "工厂到港拖车"},
		}
		for _, charge := range charges {
			if _, err := d.Charges.Create(ctx, charge); err != nil {
				return 0, fmt.Errorf("写入运杂费: %w", err)
			}
		}
	}

	if plan.customs {
		if _, err := d.Customs.Create(ctx, dto.CustomsDeclarationInput{
			ShipmentID:      shipment.ID,
			Status:          int8(model.CustomsStatusDeclared),
			DeclNo:          "310120260001",
			DeclDate:        plan.shipDate,
			SupervisionMode: "一般贸易",
			Remark:          "演示报关单，已申报",
		}); err != nil {
			return 0, fmt.Errorf("写入报关单: %w", err)
		}
	}

	return shipment.ID, nil
}

// seedPurchase 造一张以销定采的采购单、它的应付与一笔部分付款。
//
// 采购明细挂到销售订单行上（SalesOrderItemID），跟单节点才能按"这批货给谁做"排期。
func (a *App) seedPurchase(ctx context.Context, d seedDeps, m *demoMaster, order *model.SalesOrder) error {
	supplier := m.suppliers["SUP001"]
	if supplier == nil {
		return fmt.Errorf("演示数据缺少供应商档案")
	}
	detail, err := d.Orders.Get(ctx, order.ID)
	if err != nil {
		return fmt.Errorf("读取订单明细: %w", err)
	}

	inputs := make([]dto.PurchaseOrderItemInput, 0, len(detail.Items))
	for _, item := range detail.Items {
		product := m.productByID(item.ProductID)
		cost := item.UnitPrice
		if product != nil && product.CostPrice.IsPositive() {
			cost = product.CostPrice
		}
		salesItemID := item.ID
		inputs = append(inputs, dto.PurchaseOrderItemInput{
			ProductID:        item.ProductID,
			Qty:              item.Qty,
			UnitPrice:        cost,
			SalesOrderItemID: &salesItemID,
		})
	}

	po, err := d.Purchases.Create(ctx, dto.PurchaseOrderInput{
		SupplierID:   supplier.ID,
		TradeTerm:    string(model.TradeTermFOB),
		CurrencyCode: "CNY",
		OrderDate:    day(-35),
		DeliveryDate: day(-10),
		Remark:       "以销定采，" + order.DocNo,
		Items:        inputs,
	})
	if err != nil {
		return fmt.Errorf("写入采购订单: %w", err)
	}
	if _, err := d.Purchases.UpdateStatus(ctx, po.ID, dto.PurchaseOrderStatusInput{Status: int8(model.DocumentStatusConfirmed)}); err != nil {
		return fmt.Errorf("确认采购订单: %w", err)
	}

	// 跟单节点：下单已完成、生产完成逾期、验货还没做。
	if items, err := d.Purchases.Milestones(ctx, po.ID); err == nil && len(items) > 0 {
		saves := make([]dto.PurchaseMilestoneInput, 0, 3)
		for _, ms := range items {
			switch ms.Node {
			case model.PurchaseNodeOrdered:
				saves = append(saves, dto.PurchaseMilestoneInput{
					PurchaseOrderItemID: ms.PurchaseOrderItemID,
					Node:                int8(ms.Node),
					PlanDate:            day(-35),
					DoneDate:            day(-34),
				})
			case model.PurchaseNodeProduced:
				saves = append(saves, dto.PurchaseMilestoneInput{
					PurchaseOrderItemID: ms.PurchaseOrderItemID,
					Node:                int8(ms.Node),
					PlanDate:            day(-18),
				})
			case model.PurchaseNodeInspected:
				saves = append(saves, dto.PurchaseMilestoneInput{
					PurchaseOrderItemID: ms.PurchaseOrderItemID,
					Node:                int8(ms.Node),
					PlanDate:            day(-6),
				})
			}
		}
		if len(saves) > 0 {
			if _, err := d.Purchases.SaveMilestones(ctx, po.ID, dto.PurchaseMilestoneSaveInput{Milestones: saves}); err != nil {
				a.Log.Warn("演示跟单节点写入失败，跳过", "err", err)
			}
		}
	}

	payable, err := d.Payables.Create(ctx, dto.PayableInput{
		SourceType: int8(model.SettlementSourcePurchaseOrder),
		SourceID:   po.ID,
		Amount:     dec("0"),
		DueDate:    day(-18),
		Remark:     "工厂账期 30 天，已到期",
	})
	if err != nil {
		return fmt.Errorf("写入应付: %w", err)
	}

	// 部分付款：留一半未结清，账龄与待办里能看到"该付未付"。
	paid := payable.Amount.Div(dec("2")).Round(2)
	if _, err := d.Payments.Create(ctx, dto.PaymentInput{
		Direction:    20,
		PartnerID:    supplier.ID,
		PayDate:      day(-6),
		CurrencyCode: "CNY",
		Amount:       paid,
		Method:       string(model.SettlementTT),
		Reference:    "PAY2601-00312",
		Remark:       "支付一半货款",
		Allocations: []dto.PaymentAllocationInput{
			{TargetType: 20, TargetID: payable.ID, Amount: paid},
		},
	}); err != nil {
		return fmt.Errorf("写入付款: %w", err)
	}
	return nil
}

// firstClauseIDs 取条款库前 n 条的 id，供合同生成时挑几条快照下来。
func (a *App) firstClauseIDs(ctx context.Context, d seedDeps, n int) []uint {
	page, err := d.Clauses.List(ctx, dto.ClauseQuery{PageQuery: dto.PageQuery{Page: 1, PageSize: 20}})
	if err != nil || page == nil {
		return nil
	}
	ids := make([]uint, 0, n)
	for _, c := range page.List {
		if len(ids) >= n {
			break
		}
		ids = append(ids, c.ID)
	}
	return ids
}

// productByID 在演示产品里按 id 反查（装箱数据要从产品箱规推）。
func (m *demoMaster) productByID(id uint) *model.Product {
	for _, p := range m.products {
		if p.ID == id {
			return p
		}
	}
	return nil
}
