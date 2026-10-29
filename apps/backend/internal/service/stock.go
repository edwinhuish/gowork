package service

import (
	"context"
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

// StockService 管库存流水与快照。
//
// 只有一条硬规则：**快照是流水的物化结果**。这里没有任何"直接设置库存"的方法，
// 库存变化只能通过写一条流水发生；需要纠正就补正流水，或删掉录错的那条
// （删除时同事务回滚它带来的增量）。
type StockService interface {
	ListMovements(ctx context.Context, q dto.StockMovementQuery) (*dto.PageResult[model.StockMovement], error)
	GetMovement(ctx context.Context, id uint) (*model.StockMovement, error)
	CreateMovement(ctx context.Context, in dto.StockMovementInput) (*model.StockMovement, error)
	DeleteMovement(ctx context.Context, id uint) error
	ListSnapshots(ctx context.Context, q dto.StockSnapshotQuery) (*dto.PageResult[dto.StockSnapshotView], error)
	// Recalc 用流水重算快照，productID 为 nil 时重算全部。
	Recalc(ctx context.Context, productID *uint) (*dto.StockRecalcResult, error)
}

type stockService struct {
	stock    repository.StockRepository
	products repository.ProductRepository
	docNos   DocNoService
	clock    func() time.Time
	log      *slog.Logger
}

type StockOption func(*stockService)

func WithStockLogger(l *slog.Logger) StockOption {
	return func(s *stockService) { s.log = l }
}

func WithStockClock(now func() time.Time) StockOption {
	return func(s *stockService) { s.clock = now }
}

func NewStockService(
	stock repository.StockRepository,
	products repository.ProductRepository,
	docNos DocNoService,
	opts ...StockOption,
) StockService {
	s := &stockService{
		stock:    stock,
		products: products,
		docNos:   docNos,
		clock:    time.Now,
		log:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *stockService) ListMovements(
	ctx context.Context, q dto.StockMovementQuery,
) (*dto.PageResult[model.StockMovement], error) {
	q.Normalize()

	filter := repository.StockMovementFilter{
		Keyword:   strings.TrimSpace(q.Keyword),
		ProductID: q.ProductID,
	}
	if q.Type != nil {
		t := model.StockMovementType(*q.Type)
		if !t.IsValid() {
			return nil, errs.BadRequest(fmt.Sprintf("未知的库存流水类型：%d", *q.Type))
		}
		filter.Type = &t
	}
	from, err := parseOptionalDate(q.From)
	if err != nil {
		return nil, err
	}
	to, err := parseOptionalDate(q.To)
	if err != nil {
		return nil, err
	}
	filter.From = from
	filter.To = to

	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}
	list, total, err := s.stock.ListMovements(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

func (s *stockService) GetMovement(ctx context.Context, id uint) (*model.StockMovement, error) {
	return s.stock.FindMovementByID(ctx, id)
}

func (s *stockService) CreateMovement(
	ctx context.Context, in dto.StockMovementInput,
) (*model.StockMovement, error) {
	movementType := model.StockMovementType(in.Type)
	if !movementType.IsValid() {
		return nil, errs.BadRequest(fmt.Sprintf("未知的库存流水类型：%d", in.Type))
	}
	// 采购入库与销售出库必须挂在单据上：手工放行等于允许不经入库单直接加库存，
	// 那"这个数从哪来"就永远答不上来。它们由 T-16 的单据层写入。
	if !movementType.IsManual() {
		return nil, errs.BadRequest(fmt.Sprintf(
			"「%s」类流水必须由单据产生，不能手工录入", movementType.Label()))
	}

	direction := model.StockDirection(in.Direction)
	if !direction.IsValid() {
		return nil, errs.BadRequest("出入方向只能是 1（入库）或 -1（出库）")
	}

	// 数量列是 decimal(14,3)：先按 3 位定下来，别让快照累加出一个 6 位小数。
	qty := in.Qty.Round(3)
	if qty.Cmp(money.Zero()) <= 0 {
		return nil, errs.BadRequest("数量必须大于 0；方向由入库 / 出库决定，不要用负数表示出库")
	}

	occurredAt, err := parseDateOrDefault(in.OccurredAt, s.clock())
	if err != nil {
		return nil, err
	}

	product, err := s.products.FindByID(ctx, in.ProductID)
	if err != nil {
		return nil, err
	}

	docNo, err := s.docNos.Next(ctx, model.DocTypeStockMovement)
	if err != nil {
		return nil, err
	}

	m := &model.StockMovement{
		DocNo:      docNo,
		ProductID:  product.ID,
		SKUCode:    product.SKUCode,
		NameCN:     product.NameCN,
		Spec:       product.Spec,
		Unit:       product.Unit,
		Type:       movementType,
		Direction:  direction,
		Qty:        qty,
		OccurredAt: occurredAt,
		Remark:     strings.TrimSpace(in.Remark),
	}
	// 流水与快照在同一事务里写：分开写就会出现"流水有了、库存没动"的中间态。
	if err := s.stock.CreateMovement(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增库存流水", "id", m.ID, "doc_no", m.DocNo, "product_id", m.ProductID,
		"type", movementType.Label(), "direction", direction.Label(), "qty", m.Qty)
	return s.stock.FindMovementByID(ctx, m.ID)
}

func (s *stockService) DeleteMovement(ctx context.Context, id uint) error {
	m, err := s.stock.FindMovementByID(ctx, id)
	if err != nil {
		return err
	}
	// 单据产生的流水由单据回滚（T-16），这里只放行手工录入的三类。
	// 否则删掉一条销售出库流水，库存凭空多回来，而单据还挂在那里。
	if !m.Type.IsManual() {
		return errs.ErrConflict.WithMsg(fmt.Sprintf(
			"「%s」类流水由单据产生，请从来源单据处理", m.Type.Label()))
	}
	if err := s.stock.DeleteMovement(ctx, m); err != nil {
		return err
	}
	s.log.Info("删除库存流水", "id", m.ID, "doc_no", m.DocNo, "product_id", m.ProductID,
		"qty", m.SignedQty())
	return nil
}

func (s *stockService) ListSnapshots(
	ctx context.Context, q dto.StockSnapshotQuery,
) (*dto.PageResult[dto.StockSnapshotView], error) {
	q.Normalize()
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}
	rows, total, err := s.stock.ListSnapshots(ctx, strings.TrimSpace(q.Keyword), page)
	if err != nil {
		return nil, err
	}
	list := make([]dto.StockSnapshotView, 0, len(rows))
	for _, r := range rows {
		list = append(list, dto.StockSnapshotView{
			ProductID:      r.ProductID,
			SKUCode:        r.SKUCode,
			NameCN:         r.NameCN,
			Spec:           r.Spec,
			Unit:           r.Unit,
			Qty:            r.Qty.Round(3),
			LastOccurredAt: r.LastOccurredAt,
			UpdatedAt:      r.UpdatedAt,
		})
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// Recalc 用流水重算快照。
//
// 重算不是"日常操作"而是对账动作：正常情况下增量维护的快照与流水算出来的结果
// 永远相等，跑一次只会得到"没有差异"。所以这里顺手把重算前的不一致记下来——
// 它一旦出现，说明有人绕过了流水改过快照，这条信息比"重算成功"本身更有用。
func (s *stockService) Recalc(
	ctx context.Context, productID *uint,
) (*dto.StockRecalcResult, error) {
	ids := make([]uint, 0, 16)
	if productID != nil {
		if _, err := s.products.FindByID(ctx, *productID); err != nil {
			return nil, err
		}
		ids = append(ids, *productID)
	} else {
		all, err := s.stock.ProductsWithMovements(ctx)
		if err != nil {
			return nil, err
		}
		ids = append(ids, all...)
	}

	result := &dto.StockRecalcResult{Mismatched: make([]dto.StockRecalcDiff, 0)}
	for _, id := range ids {
		before, err := s.stock.FindSnapshot(ctx, id)
		if err != nil {
			return nil, err
		}
		sum, err := s.stock.SumMovements(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := s.stock.ReplaceSnapshot(ctx, id, sum); err != nil {
			return nil, err
		}
		result.Recalculated++

		// 还没有快照行时按 0 比：这个产品第一次重算不该被算成"差异"。
		snapshotQty := money.Zero()
		if before != nil {
			snapshotQty = before.Qty.Round(3)
		}
		if !snapshotQty.Sub(sum.Qty).Round(3).IsZero() {
			result.Mismatched = append(result.Mismatched, dto.StockRecalcDiff{
				ProductID:   id,
				SnapshotQty: snapshotQty,
				MovementQty: sum.Qty,
			})
		}
	}
	if len(result.Mismatched) > 0 {
		s.log.Warn("库存快照与流水不一致，已按流水修正",
			"products", len(result.Mismatched), "recalculated", result.Recalculated)
	}
	return result, nil
}
