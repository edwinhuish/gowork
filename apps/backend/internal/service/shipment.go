package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// ShipmentService 管出货批次：一票出货装哪些销售订单行、各多少。
//
// 这个模块第一次出现"同一份数量被两张单据同时占用"的问题：
// 销售订单行上的 qty 是答应客户的数量，"还能出多少"要看它已经被多少票分走了。
// 所以数量校验必须落在分配行上、在服务端算，界面上拦不住的（两个页面同时开着就够了）。
type ShipmentService interface {
	Create(ctx context.Context, in dto.ShipmentInput) (*model.Shipment, error)
	Update(ctx context.Context, id uint, in dto.ShipmentInput) (*model.Shipment, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Shipment, error)
	List(ctx context.Context, q dto.ShipmentQuery) (*dto.PageResult[model.Shipment], error)
	UpdateStatus(ctx context.Context, id uint, in dto.ShipmentStatusInput) (*model.Shipment, error)
	// UpdateLogistics 只改提单号、船名航次与 ETD / ETA，任何状态都能改。
	UpdateLogistics(
		ctx context.Context, id uint, in dto.ShipmentLogisticsInput,
	) (*model.Shipment, error)
	// Reissue 改证重开：号码不变，商票版本加一（文件名与打印件带 -R{n}）。
	Reissue(ctx context.Context, id uint) (*model.Shipment, error)
	// Candidates 列出还能出货的销售订单行，供出货单选行。
	// excludeShipmentID 传当前正在编辑的那一票，好把它自己占的数量还给它。
	Candidates(ctx context.Context, keyword string, excludeShipmentID uint) ([]dto.ShipmentCandidate, error)
}

// candidateLimit 限制可出货行的返回条数。出货单选行是一个下拉，
// 一次给出几千行既卡界面也用不上——要找具体的单用关键字搜单号 / SKU。
const candidateLimit = 200

type shipmentService struct {
	shipments repository.ShipmentRepository
	salesOrds repository.SalesOrderRepository
	// charges 与 customs 只在删除前一票时用：删掉批次会让它下面的费用与报关记录
	// 变成查不到来源的孤儿行。
	charges repository.ShipmentChargeRepository
	customs repository.CustomsDeclarationRepository
	docNos  DocNoService
	clock   func() time.Time
	log     *slog.Logger
}

type ShipmentOption func(*shipmentService)

func WithShipmentLogger(l *slog.Logger) ShipmentOption {
	return func(s *shipmentService) { s.log = l }
}

// WithShipmentClock 注入时钟：出货日默认值与单号年月要可断言。
func WithShipmentClock(now func() time.Time) ShipmentOption {
	return func(s *shipmentService) { s.clock = now }
}

func NewShipmentService(
	shipments repository.ShipmentRepository,
	salesOrds repository.SalesOrderRepository,
	charges repository.ShipmentChargeRepository,
	customs repository.CustomsDeclarationRepository,
	docNos DocNoService,
	opts ...ShipmentOption,
) ShipmentService {
	s := &shipmentService{
		shipments: shipments,
		salesOrds: salesOrds,
		charges:   charges,
		customs:   customs,
		docNos:    docNos,
		clock:     time.Now,
		log:       slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *shipmentService) Create(
	ctx context.Context, in dto.ShipmentInput,
) (*model.Shipment, error) {
	shipDate, err := s.parseShipDate(in.ShipDate)
	if err != nil {
		return nil, err
	}
	rows, err := s.buildAllocations(ctx, in.Allocations, 0)
	if err != nil {
		return nil, err
	}

	docNo, err := s.docNos.Next(ctx, model.DocTypeShipment)
	if err != nil {
		return nil, err
	}

	m := &model.Shipment{
		DocNo:    docNo,
		Status:   model.DocumentStatusDraft,
		ShipDate: shipDate,
		Remark:   in.Remark,
	}
	if err := s.shipments.CreateWithAllocations(ctx, m, rows); err != nil {
		return nil, err
	}
	s.log.Info("新增出货批次", "id", m.ID, "doc_no", m.DocNo, "rows", len(rows))
	return s.load(ctx, m.ID)
}

func (s *shipmentService) Update(
	ctx context.Context, id uint, in dto.ShipmentInput,
) (*model.Shipment, error) {
	existing, err := s.shipments.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !existing.IsEditable() {
		return nil, errs.ErrConflict.WithMsg(
			"只有草稿可以修改；已确认的出货单仓库与货代已经在按它备货")
	}
	shipDate, err := s.parseShipDate(in.ShipDate)
	if err != nil {
		return nil, err
	}
	// 排除自己再统计已出数量：否则"把 100 改成 90"会因为原来的 100 已经占着而永远超量。
	rows, err := s.buildAllocations(ctx, in.Allocations, id)
	if err != nil {
		return nil, err
	}
	fields := map[string]any{
		"ship_date": shipDate,
		"remark":    in.Remark,
	}
	if in.UpdatedAt != nil {
		fields[repository.FieldUpdatedAt] = *in.UpdatedAt
	}
	if err := s.shipments.UpdateWithAllocations(ctx, id, fields, rows); err != nil {
		return nil, err
	}
	return s.load(ctx, id)
}

func (s *shipmentService) Delete(ctx context.Context, id uint) error {
	existing, err := s.shipments.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !existing.IsEditable() {
		return errs.ErrConflict.WithMsg("只有草稿可以删除；已确认的出货单请改为已取消")
	}
	// 费用行不跟着批次走：批次软删除后，那些费用仍留在库里却再也点不进来，
	// 对账时会出现"这笔钱是给哪票货的"查不到的情况。与产品↔货源同一条规则。
	n, err := s.charges.CountByShipmentID(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errs.ErrConflict.WithMsg(
			fmt.Sprintf("该出货批次还有 %d 条运费杂费，请先删除费用", n))
	}
	m, err := s.customs.CountByShipmentID(ctx, id)
	if err != nil {
		return err
	}
	if m > 0 {
		return errs.ErrConflict.WithMsg(
			fmt.Sprintf("该出货批次还有 %d 张报关单，请先删除报关记录", m))
	}
	return s.shipments.Delete(ctx, id)
}

func (s *shipmentService) Get(ctx context.Context, id uint) (*model.Shipment, error) {
	return s.load(ctx, id)
}

func (s *shipmentService) List(
	ctx context.Context, q dto.ShipmentQuery,
) (*dto.PageResult[model.Shipment], error) {
	q.PageQuery.Normalize()

	filter := repository.ShipmentFilter{
		Keyword:      q.Keyword,
		SalesOrderID: q.SalesOrderID,
	}
	if q.Status != nil {
		st := model.DocumentStatus(*q.Status)
		if !st.IsValid() {
			return nil, errs.BadRequest("status 不合法")
		}
		filter.Status = &st
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.shipments.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	for i := range list {
		annotateDocNo(&list[i])
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// Candidates 只做"查出来 + 算未出数量"，不在这里再筛一遍：
// 能不能出、还剩多少由仓储的同一条 SQL 一次算清，两处各算一遍就会对不上。
func (s *shipmentService) Candidates(
	ctx context.Context, keyword string, excludeShipmentID uint,
) ([]dto.ShipmentCandidate, error) {
	items, err := s.shipments.ListShippableItems(ctx, keyword, excludeShipmentID, candidateLimit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ShipmentCandidate, 0, len(items))
	for _, it := range items {
		c := dto.ShipmentCandidate{
			SalesOrderID:     it.SalesOrderID,
			SalesOrderDocNo:  it.SalesOrderDocNo,
			PartnerName:      it.PartnerName,
			SalesOrderItemID: it.SalesOrderItemID,
			SKUCode:          it.SKUCode,
			NameCN:           it.NameCN,
			Spec:             it.Spec,
			Unit:             it.Unit,
			Qty:              it.Qty.Round(3),
			ShippedQty:       it.ShippedQty.Round(3),
			UnshippedQty:     it.Qty.Sub(it.ShippedQty).Round(3),
		}
		if it.DeliveryDate != nil {
			c.DeliveryDate = it.DeliveryDate.Format(rateDateLayout)
		}
		out = append(out, c)
	}
	return out, nil
}

// load 取回一整票并补上摊平的来源单号，供详情与新建 / 编辑后回吐。
func (s *shipmentService) load(ctx context.Context, id uint) (*model.Shipment, error) {
	m, err := s.shipments.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	annotateDocNo(m)
	return m, nil
}

// annotateDocNo 把每条分配行背后的订单号摊平出来。
//
// 订单号本来在 allocations[].sales_order_item.sales_order 里，但那一层整单对象
// 是不回吐接口的（json:"-"），前端拿不到；让前端自己去查第二遍又会让列表多几十个请求。
func annotateDocNo(m *model.Shipment) {
	if m == nil {
		return
	}
	for i := range m.Allocations {
		item := m.Allocations[i].SalesOrderItem
		if item != nil && item.SalesOrder != nil {
			m.Allocations[i].SalesOrderDocNo = item.SalesOrder.DocNo
		}
	}
}

func (s *shipmentService) UpdateStatus(
	ctx context.Context, id uint, in dto.ShipmentStatusInput,
) (*model.Shipment, error) {
	existing, err := s.shipments.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	next := model.DocumentStatus(in.Status)
	if !next.IsValid() {
		return nil, errs.BadRequest("status 不合法")
	}
	if err := checkDocumentTransit(existing.Status, next); err != nil {
		return nil, err
	}
	if err := s.shipments.Update(ctx, id, map[string]any{"status": next}); err != nil {
		return nil, err
	}
	s.log.Info("出货批次状态变更", "id", id, "from", existing.Status, "to", next)
	return s.shipments.FindByID(ctx, id)
}

// UpdateLogistics 只改物流节点，任何状态都能改。
//
// 与 Update 分开：出货单的数量在草稿阶段就定稿了，改它要让仓库与货代重做一遍；
// 而提单号、船名航次、ETD / ETA 本来就是订舱之后才有的，那时单子早已确认。
// 把两件事捆在同一个入口，结果是"想记一个提单号，得先有权改整张单"。
func (s *shipmentService) UpdateLogistics(
	ctx context.Context, id uint, in dto.ShipmentLogisticsInput,
) (*model.Shipment, error) {
	if _, err := s.shipments.FindByID(ctx, id); err != nil {
		return nil, err
	}

	etd, err := parseOptionalDateOnly(in.ETD)
	if err != nil {
		return nil, errs.BadRequest("ETD 格式应为 YYYY-MM-DD")
	}
	eta, err := parseOptionalDateOnly(in.ETA)
	if err != nil {
		return nil, errs.BadRequest("ETA 格式应为 YYYY-MM-DD")
	}
	// 到港早于开船一定是录错了。不拦的话它会安静地存在，
	// 直到有人按这两个日期排提货，才发现货"在昨天就已经到港"。
	if etd != nil && eta != nil && eta.Before(*etd) {
		return nil, errs.BadRequest("ETA 不能早于 ETD")
	}

	fields := map[string]any{
		"bl_no":  strings.TrimSpace(in.BLNo),
		"vessel": strings.TrimSpace(in.Vessel),
		"etd":    etd,
		"eta":    eta,
	}
	if err := s.shipments.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	s.log.Info("更新物流节点", "id", id, "bl_no", in.BLNo, "vessel", in.Vessel)
	return s.load(ctx, id)
}

// Reissue 改证重开：号码不变，商票版本加一。
//
// 换号等于换了一票货——报关单、提单与客户账上都认原号，所以版本只体现在文件名
// 与打印件上（`DN-2610-001-R1`）。草稿不需要改证：内容还能直接改；
// 已取消的票也不再出单证。
func (s *shipmentService) Reissue(ctx context.Context, id uint) (*model.Shipment, error) {
	existing, err := s.shipments.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	switch existing.Status {
	case model.DocumentStatusDraft:
		return nil, errs.ErrConflict.WithMsg("草稿还没有签发，直接改内容即可，不需要改证")
	case model.DocumentStatusCancelled:
		return nil, errs.ErrConflict.WithMsg("已取消的出货单不再出单证，不需要改证")
	}

	rev, err := s.shipments.BumpInvoiceRevision(ctx, id)
	if err != nil {
		return nil, err
	}
	s.log.Info("商票改证重开", "id", id, "doc_no", existing.DocNo, "revision", rev)
	return s.load(ctx, id)
}

func (s *shipmentService) parseShipDate(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return model.DateOnly(s.clock()), nil
	}
	return parseDateOnly(raw)
}

// buildAllocations 校验每一行并生成分配行。
//
// 三条约束：销售订单必须处在"可以出货"的状态；数量不得超过未出数量；
// 同一次请求里重复提交同一行要合并计算——否则拆成两行各填一半就能绕过校验。
func (s *shipmentService) buildAllocations(
	ctx context.Context, in []dto.ShipmentAllocationInput, excludeShipmentID uint,
) ([]model.ShipmentAllocation, error) {
	rows := make([]model.ShipmentAllocation, 0, len(in))
	// used 记录本次请求里已经给每一行分配了多少，用于捕获同请求内的重复行。
	used := make(map[uint]money.Decimal, len(in))
	// dealPartnerID 记住这票货的客户：一票只能装同一个客户的订单。
	// 货代可以拼柜，单据不能拼——送货单、商业发票、装箱单上只有一个收货人。
	var dealPartnerID uint
	// currencyCode 同理：商业发票上只有一个币种与一个合计。
	// 混币种的订单并进一票，发票要么合价说谎、要么两行之间无法相加。
	var currencyCode string

	for _, row := range in {
		// 数量列是 3 位小数：先把入参定下来，再拿去和"还能出多少"比。
		row.Qty = row.Qty.Round(3)
		row.GrossWeight = row.GrossWeight.Round(3)
		row.NetWeight = row.NetWeight.Round(3)
		if row.Qty.Cmp(money.Zero()) <= 0 {
			return nil, errs.BadRequest("出货数量必须大于 0")
		}
		item, err := s.salesOrds.FindItemByID(ctx, row.SalesOrderItemID)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				return nil, errs.BadRequest("销售订单行不存在")
			}
			return nil, err
		}
		if err := checkOrderShippable(item); err != nil {
			return nil, err
		}

		// 跨客户合并一票：报错而不是悄悄合并。合并之后这票的商业发票开给谁？
		// 拆成两票各开一张才是对的，而"拆"必须是用户的选择。
		if dealPartnerID == 0 {
			dealPartnerID = item.SalesOrder.PartnerID
		} else if item.SalesOrder.PartnerID != dealPartnerID {
			return nil, errs.BadRequest(fmt.Sprintf(
				"一票货只能装同一个客户的订单：%s 属于另一个客户", item.SalesOrder.DocNo))
		}

		// 币种同理：商业发票只有一个币种与一个合计。分币种拆票，别在一张发票上混着开。
		if currencyCode == "" {
			currencyCode = item.SalesOrder.CurrencyCode
		} else if item.SalesOrder.CurrencyCode != currencyCode {
			return nil, errs.BadRequest(fmt.Sprintf(
				"一票货只能装同一币种的订单：%s 是 %s，本票已按 %s 开票",
				item.SalesOrder.DocNo, item.SalesOrder.CurrencyCode, currencyCode))
		}

		allocated, err := s.shipments.AllocatedQty(ctx, item.ID, excludeShipmentID)
		if err != nil {
			return nil, err
		}
		remaining := item.Qty.Sub(allocated).Sub(used[item.ID]).Round(3)
		if row.Qty.Cmp(remaining) > 0 {
			if remaining.IsNegative() {
				remaining = money.Zero()
			}
			return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
				"%s 的 %s 最多还能出 %s，本次填了 %s",
				orderDocNo(item), item.SKUCode,
				formatQty(remaining), formatQty(row.Qty)))
		}
		used[item.ID] = used[item.ID].Add(row.Qty)

		if err := checkPacking(item, row); err != nil {
			return nil, err
		}

		rows = append(rows, model.ShipmentAllocation{
			SalesOrderItemID: item.ID,
			SKUCode:          item.SKUCode,
			NameCN:           item.NameCN,
			Spec:             item.Spec,
			Unit:             item.Unit,
			Qty:              row.Qty,
			Remark:           row.Remark,
			Cartons:          row.Cartons,
			GrossWeight:      row.GrossWeight,
			NetWeight:        row.NetWeight,
			Measurement:      strings.TrimSpace(row.Measurement),
		})
	}
	return rows, nil
}

// checkPacking 校验一行装箱数据。
//
// 只拦"一定录反了"的情形，不要求填全：装箱数据是装箱时才有的，
// 草稿阶段留空很正常，逼着填只会让人随手填 0。
func checkPacking(item *model.SalesOrderItem, row dto.ShipmentAllocationInput) error {
	if row.Cartons < 0 || row.GrossWeight.IsNegative() || row.NetWeight.IsNegative() {
		return errs.BadRequest(item.SKUCode + " 的箱数与毛净重不能为负")
	}
	// 上限沿用 1000 吨：一行明细装不了更重，多半是把"单箱重量 × 箱数"算重了。
	if row.GrossWeight.Cmp(money.FromInt(1_000_000)) > 0 ||
		row.NetWeight.Cmp(money.FromInt(1_000_000)) > 0 {
		return errs.BadRequest(item.SKUCode + " 的毛净重超出可录入范围")
	}
	// 净重是货物本身，毛重是货物加包装：净重大于毛重只可能是两栏填反了。
	// 装箱单上这两个数是要申报的，印出去再改就是改证。
	if row.NetWeight.Cmp(row.GrossWeight) > 0 {
		return errs.BadRequest(fmt.Sprintf(
			"%s 的净重 %s 大于毛重 %s，两栏可能填反了",
			item.SKUCode, formatQty(row.NetWeight), formatQty(row.GrossWeight)))
	}
	return nil
}

// checkOrderShippable 判断这张销售订单现在能不能出货。
//
// 草稿不能出：还没对客户承诺就发货，说明流程倒了（客户可能根本不认这单）。
// 已取消、已完成更不能出。只剩已确认与备货中两种状态是"该出还没出"。
func checkOrderShippable(item *model.SalesOrderItem) error {
	if item.SalesOrder == nil {
		return errs.BadRequest("销售订单行缺少所属订单，无法校验出货条件")
	}
	switch item.SalesOrder.Status {
	case model.DocumentStatusConfirmed, model.DocumentStatusPreparing:
		return nil
	case model.DocumentStatusDraft:
		return errs.ErrConflict.WithMsg(orderDocNo(item) + " 还是草稿，请先确认订单再出货")
	case model.DocumentStatusCancelled:
		return errs.ErrConflict.WithMsg(orderDocNo(item) + " 已取消，不能出货")
	default:
		return errs.ErrConflict.WithMsg(orderDocNo(item) + " 已完成出货，不能再出货")
	}
}

func orderDocNo(item *model.SalesOrderItem) string {
	if item.SalesOrder == nil {
		return "销售订单"
	}
	return item.SalesOrder.DocNo
}

// formatQty 按 3 位小数取整再去掉末尾的 0：出货数量多数是整数，
// "800.000" 读起来像有小数。
func formatQty(v money.Decimal) string { return v.Round(3).String() }
