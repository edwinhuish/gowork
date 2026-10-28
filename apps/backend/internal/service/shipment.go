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
	// Candidates 列出还能出货的销售订单行，供出货单选行。
	// excludeShipmentID 传当前正在编辑的那一票，好把它自己占的数量还给它。
	Candidates(ctx context.Context, keyword string, excludeShipmentID uint) ([]dto.ShipmentCandidate, error)
}

// qtyEpsilon 是数量比较的容差。qty 存 decimal(14,3)，加减之后会有 1e-13 级别的误差，
// 直接用 > 比较会把"刚好出完"判成超量。
const qtyEpsilon = 1e-6

// candidateLimit 限制可出货行的返回条数。出货单选行是一个下拉，
// 一次给出几千行既卡界面也用不上——要找具体的单用关键字搜单号 / SKU。
const candidateLimit = 200

type shipmentService struct {
	shipments repository.ShipmentRepository
	salesOrds repository.SalesOrderRepository
	docNos    DocNoService
	clock     func() time.Time
	log       *slog.Logger
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
	docNos DocNoService,
	opts ...ShipmentOption,
) ShipmentService {
	s := &shipmentService{
		shipments: shipments,
		salesOrds: salesOrds,
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
			Qty:              it.Qty,
			ShippedQty:       money.Round(it.ShippedQty, 3),
			UnshippedQty:     money.Round(it.Qty-it.ShippedQty, 3),
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
	used := make(map[uint]float64, len(in))

	for _, row := range in {
		if row.Qty <= 0 {
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

		allocated, err := s.shipments.AllocatedQty(ctx, item.ID, excludeShipmentID)
		if err != nil {
			return nil, err
		}
		remaining := item.Qty - allocated - used[item.ID]
		if row.Qty > remaining+qtyEpsilon {
			return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
				"%s 的 %s 最多还能出 %s，本次填了 %s",
				orderDocNo(item), item.SKUCode,
				formatQty(max(remaining, 0)), formatQty(row.Qty)))
		}
		used[item.ID] += row.Qty

		rows = append(rows, model.ShipmentAllocation{
			SalesOrderItemID: item.ID,
			SKUCode:          item.SKUCode,
			NameCN:           item.NameCN,
			Spec:             item.Spec,
			Unit:             item.Unit,
			Qty:              row.Qty,
			Remark:           row.Remark,
		})
	}
	return rows, nil
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

// formatQty 去掉末尾的 0：出货数量多数是整数，"800.000" 读起来像有小数。
func formatQty(v float64) string {
	r := money.Round(v, 3)
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", r), "0"), ".")
}
