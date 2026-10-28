package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// StockDocumentService 管出入库单。
//
// 单据与流水的关系是单向的：**单据确认才产生流水**，流水从不反向改单据。
// 草稿阶段怎么改都不影响库存；一旦确认，单据就冻结——要纠正只能做一张反向单。
type StockDocumentService interface {
	List(ctx context.Context, q dto.StockDocumentQuery) (*dto.PageResult[model.StockDocument], error)
	Get(ctx context.Context, id uint) (*model.StockDocument, error)
	Create(ctx context.Context, in dto.StockDocumentInput) (*model.StockDocument, error)
	Update(ctx context.Context, id uint, in dto.StockDocumentInput) (*model.StockDocument, error)
	Delete(ctx context.Context, id uint) error
	// Confirm 确认单据：按类型生成库存流水并更新快照。
	Confirm(ctx context.Context, id uint) (*model.StockDocument, error)
}

type stockDocumentService struct {
	docs     repository.StockDocumentRepository
	products repository.ProductRepository
	partners repository.PartnerRepository
	docNos   DocNoService
	clock    func() time.Time
	log      *slog.Logger
}

type StockDocumentOption func(*stockDocumentService)

func WithStockDocumentLogger(l *slog.Logger) StockDocumentOption {
	return func(s *stockDocumentService) { s.log = l }
}

func WithStockDocumentClock(now func() time.Time) StockDocumentOption {
	return func(s *stockDocumentService) { s.clock = now }
}

func NewStockDocumentService(
	docs repository.StockDocumentRepository,
	products repository.ProductRepository,
	partners repository.PartnerRepository,
	docNos DocNoService,
	opts ...StockDocumentOption,
) StockDocumentService {
	s := &stockDocumentService{
		docs:     docs,
		products: products,
		partners: partners,
		docNos:   docNos,
		clock:    time.Now,
		log:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *stockDocumentService) List(
	ctx context.Context, q dto.StockDocumentQuery,
) (*dto.PageResult[model.StockDocument], error) {
	q.Normalize()

	filter := repository.StockDocumentFilter{
		Keyword:   strings.TrimSpace(q.Keyword),
		PartnerID: q.PartnerID,
	}
	if q.Type != nil {
		t := model.StockDocType(*q.Type)
		if !t.IsValid() {
			return nil, errs.BadRequest(fmt.Sprintf("未知的出入库单类型：%d", *q.Type))
		}
		filter.Type = &t
	}
	if q.Status != nil {
		st := model.DocumentStatus(*q.Status)
		if !st.IsValid() {
			return nil, errs.BadRequest(fmt.Sprintf("未知的单据状态：%d", *q.Status))
		}
		filter.Status = &st
	}

	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}
	list, total, err := s.docs.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

func (s *stockDocumentService) Get(ctx context.Context, id uint) (*model.StockDocument, error) {
	return s.docs.FindByID(ctx, id)
}

func (s *stockDocumentService) Create(
	ctx context.Context, in dto.StockDocumentInput,
) (*model.StockDocument, error) {
	t, err := resolveStockDocType(in.Type)
	if err != nil {
		return nil, err
	}
	docDate, err := parseDateOrDefault(in.DocDate, s.clock())
	if err != nil {
		return nil, err
	}
	items, err := s.buildItems(ctx, t, in.Items)
	if err != nil {
		return nil, err
	}
	if err := s.checkPartner(ctx, in.PartnerID); err != nil {
		return nil, err
	}

	docNo, err := s.docNos.Next(ctx, t.DocType())
	if err != nil {
		return nil, err
	}

	doc := &model.StockDocument{
		DocNo:       docNo,
		Type:        t,
		Status:      model.DocumentStatusDraft,
		DocDate:     docDate,
		PartnerID:   in.PartnerID,
		SourceDocNo: strings.TrimSpace(in.SourceDocNo),
		Remark:      strings.TrimSpace(in.Remark),
	}
	if err := s.docs.CreateWithItems(ctx, doc, items); err != nil {
		return nil, err
	}
	s.log.Info("新增出入库单", "id", doc.ID, "doc_no", doc.DocNo,
		"type", t.Label(), "rows", len(items))
	return s.docs.FindByID(ctx, doc.ID)
}

func (s *stockDocumentService) Update(
	ctx context.Context, id uint, in dto.StockDocumentInput,
) (*model.StockDocument, error) {
	existing, err := s.docs.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !existing.IsEditable() {
		return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
			"「%s」已确认，不能再改；要纠正请做一张反向单", existing.DocNo))
	}
	// 类型不可改：单号前缀已经按类型发出去了，改类型等于凭空多出一张对不上号的单。
	if int8(existing.Type) != in.Type {
		return nil, errs.BadRequest("单据类型不可修改；要换类型请新建一张")
	}
	t := existing.Type
	docDate, err := parseDateOrDefault(in.DocDate, s.clock())
	if err != nil {
		return nil, err
	}
	items, err := s.buildItems(ctx, t, in.Items)
	if err != nil {
		return nil, err
	}
	if err := s.checkPartner(ctx, in.PartnerID); err != nil {
		return nil, err
	}

	fields := map[string]any{
		"doc_date":      docDate,
		"partner_id":    in.PartnerID,
		"source_doc_no": strings.TrimSpace(in.SourceDocNo),
		"remark":        strings.TrimSpace(in.Remark),
	}
	if err := s.docs.UpdateWithItems(ctx, id, fields, items); err != nil {
		return nil, err
	}
	s.log.Info("修改出入库单", "id", id, "doc_no", existing.DocNo, "rows", len(items))
	return s.docs.FindByID(ctx, id)
}

func (s *stockDocumentService) Delete(ctx context.Context, id uint) error {
	existing, err := s.docs.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !existing.IsEditable() {
		return errs.ErrConflict.WithMsg(fmt.Sprintf(
			"「%s」已确认，流水已经落地，不能删除；要纠正请做一张反向单", existing.DocNo))
	}
	if err := s.docs.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("删除出入库单", "id", id, "doc_no", existing.DocNo)
	return nil
}

// Confirm 确认单据，按类型生成流水。
//
// 三类单据的差别都收在 resolvePosting 里：
//
//	采购入库 / 销售出库：方向固定，每行一条流水，数量照抄；
//	盘点：方向与数量由「实盘 − 账面」决定，差异为 0 的行不产生流水。
//
// 账面数一律取确认那一刻的快照，而不是草稿建立时的值：
// 盘点单可能搁置半天，这中间别的单据动过库存，按旧账面算出来的盈亏就是错的。
func (s *stockDocumentService) Confirm(
	ctx context.Context, id uint,
) (*model.StockDocument, error) {
	doc, err := s.docs.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !doc.IsEditable() {
		return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
			"「%s」当前是「%s」，只有草稿可以确认", doc.DocNo, doc.Status.Label()))
	}
	if err := checkDocumentTransit(doc.Status, model.DocumentStatusConfirmed); err != nil {
		return nil, err
	}

	items := make([]model.StockDocumentItem, 0, len(doc.Items))
	movements := make([]model.StockMovement, 0, len(doc.Items))

	for _, item := range doc.Items {
		direction, qty, bookQty, err := s.resolvePosting(ctx, doc, item)
		if err != nil {
			return nil, err
		}
		item.BookQty = bookQty
		items = append(items, item)

		// 盘盈盘亏为 0：这一行没有发生任何库存变动，不该留下一条数量为 0 的流水。
		if qty <= 0 {
			continue
		}
		mvNo, err := s.docNos.Next(ctx, model.DocTypeStockMovement)
		if err != nil {
			return nil, err
		}
		movements = append(movements, model.StockMovement{
			DocNo:       mvNo,
			ProductID:   item.ProductID,
			SKUCode:     item.SKUCode,
			NameCN:      item.NameCN,
			Spec:        item.Spec,
			Unit:        item.Unit,
			Type:        doc.Type.MovementType(),
			Direction:   direction,
			Qty:         qty,
			OccurredAt:  doc.DocDate,
			SourceType:  model.StockSourceStockDocument,
			SourceID:    doc.ID,
			SourceDocNo: doc.DocNo,
			Remark:      item.Remark,
		})
	}

	if err := s.docs.Post(ctx, doc.ID, s.clock(), items, movements); err != nil {
		return nil, err
	}
	s.log.Info("出入库单已确认", "id", doc.ID, "doc_no", doc.DocNo,
		"type", doc.Type.Label(), "movements", len(movements))
	return s.docs.FindByID(ctx, doc.ID)
}

// resolvePosting 算出一行明细该产生什么方向、多少数量的流水。
//
// 盘点时返回的 bookQty 要回写到明细上：确认之后再看这张单，
// 必须能回答"当时账上是多少"，否则差异没有依据。
func (s *stockDocumentService) resolvePosting(
	ctx context.Context, doc *model.StockDocument, item model.StockDocumentItem,
) (model.StockDirection, float64, *float64, error) {
	if fixed, ok := doc.Type.FixedDirection(); ok {
		return fixed, item.Qty, nil, nil
	}

	bookQty, err := s.docs.CurrentQty(ctx, item.ProductID)
	if err != nil {
		return 0, 0, nil, err
	}
	book := money.Round(bookQty, 3)
	diff := money.Round(item.Qty-book, 3)
	if math.Abs(diff) <= qtyEpsilon {
		// 账实相符：方向与数量都给 0，调用方据此跳过流水。
		return 0, 0, &book, nil
	}
	if diff > 0 {
		return model.StockDirectionIn, math.Abs(diff), &book, nil
	}
	return model.StockDirectionOut, math.Abs(diff), &book, nil
}

func (s *stockDocumentService) buildItems(
	ctx context.Context, t model.StockDocType, rows []dto.StockDocumentItemInput,
) ([]model.StockDocumentItem, error) {
	items := make([]model.StockDocumentItem, 0, len(rows))
	for _, row := range rows {
		qty := money.Round(*row.Qty, 3)
		if qty < 0 {
			return nil, errs.BadRequest("数量不能是负数；方向由单据类型决定")
		}
		// 盘点允许 0（盘到没有货是结论），入库 / 出库填 0 说明这行没写完。
		if t != model.StockDocCount && qty <= 0 {
			return nil, errs.BadRequest("数量必须大于 0")
		}
		product, err := s.products.FindByID(ctx, row.ProductID)
		if err != nil {
			return nil, err
		}
		items = append(items, model.StockDocumentItem{
			ProductID: product.ID,
			SKUCode:   product.SKUCode,
			NameCN:    product.NameCN,
			Spec:      product.Spec,
			Unit:      product.Unit,
			Qty:       qty,
			Remark:    strings.TrimSpace(row.Remark),
		})
	}
	return items, nil
}

func (s *stockDocumentService) checkPartner(ctx context.Context, partnerID *uint) error {
	if partnerID == nil {
		return nil
	}
	_, err := s.partners.FindByID(ctx, *partnerID)
	return err
}

// resolveStockDocType 把入参里的类型码变成可用的类型。
func resolveStockDocType(raw int8) (model.StockDocType, error) {
	t := model.StockDocType(raw)
	if !t.IsValid() {
		return 0, errs.BadRequest(fmt.Sprintf("未知的出入库单类型：%d", raw))
	}
	if !t.IsImplemented() {
		return 0, errs.BadRequest(fmt.Sprintf(
			"「%s」要等仓库维度落地后才能用（见 docs/roadmap.md T-16b）", t.Label()))
	}
	return t, nil
}
