package service

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/pkg/money"
	"github.com/edwinhuish/gowork/internal/repository"
)

// PurchaseReceiptService 管采购收货单（T-51a）。
//
// 它回答三个此前答不出的问题：还有多少没到（在途量）、这批货是哪张采购单的、
// 入库的那批数与收货时说的一致吗。所以确认动作必须同时做两件事：
// 置状态、生成采购入库单——两件不在一个事务里，就会出现「收货单已确认、
// 库存没动」的中间态。
type PurchaseReceiptService interface {
	// Summary 给采购详情用：收货单列表 + 每行已收量。
	Summary(ctx context.Context, orderID uint) (*dto.PurchaseReceiptSummary, error)
	Get(ctx context.Context, id uint) (*model.PurchaseReceipt, error)
	Create(ctx context.Context, orderID uint, in dto.PurchaseReceiptInput) (*model.PurchaseReceipt, error)
	// Confirm 置为已确认，并在同一事务里生成采购入库单并确认它。
	Confirm(ctx context.Context, id uint) (*model.PurchaseReceipt, error)
	Delete(ctx context.Context, id uint) error
}

type purchaseReceiptService struct {
	receipts repository.PurchaseReceiptRepository
	orders   repository.PurchaseOrderRepository
	docNos   DocNoService
	txs      repository.TxManager
	stockIn  PurchaseInIssuer
	log      *slog.Logger
}

// PurchaseInIssuer 是收货确认要用的那一件事：给我入库。
//
// 与 SalesOutIssuer 同一思路：调用方只需要「给我入库」，
// 不需要知道出入库单怎么建、流水怎么写、快照怎么更新。
type PurchaseInIssuer interface {
	IssuePurchaseIn(ctx context.Context, in dto.PurchaseInIssueInput) (*model.StockDocument, error)
}

type PurchaseReceiptOption func(*purchaseReceiptService)

func WithPurchaseReceiptLogger(l *slog.Logger) PurchaseReceiptOption {
	return func(s *purchaseReceiptService) { s.log = l }
}

func NewPurchaseReceiptService(
	receipts repository.PurchaseReceiptRepository,
	orders repository.PurchaseOrderRepository,
	docNos DocNoService,
	txs repository.TxManager,
	stockIn PurchaseInIssuer,
	opts ...PurchaseReceiptOption,
) PurchaseReceiptService {
	s := &purchaseReceiptService{
		receipts: receipts, orders: orders, docNos: docNos, txs: txs, stockIn: stockIn,
		log: slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *purchaseReceiptService) Summary(
	ctx context.Context, orderID uint,
) (*dto.PurchaseReceiptSummary, error) {
	if _, err := s.orders.FindByID(ctx, orderID); err != nil {
		return nil, err
	}
	list, err := s.receipts.ListByOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	received := s.receivedByItem(list)
	out := make(map[string]string, len(received))
	for id, qty := range received {
		out[strconv.FormatUint(uint64(id), 10)] = qty.String()
	}
	return &dto.PurchaseReceiptSummary{Receipts: list, Received: out}, nil
}

func (s *purchaseReceiptService) Get(ctx context.Context, id uint) (*model.PurchaseReceipt, error) {
	return s.receipts.FindByID(ctx, id)
}

func (s *purchaseReceiptService) Create(
	ctx context.Context, orderID uint, in dto.PurchaseReceiptInput,
) (*model.PurchaseReceipt, error) {
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	// 草稿单不该有收货：还没对供应商下单，货不可能已经到了。
	// 已取消同理——那张单上的货不该再往库里进。
	if order.Status == model.DocumentStatusDraft || order.Status == model.DocumentStatusCancelled {
		return nil, errs.ErrConflict.WithMsg("只有已确认的采购单才能收货")
	}

	// 到货日留空取今天：补录昨天的到货很常见，而不填就报错会让人随手编一个。
	receivedAt := model.DateOnly(time.Now())
	if strings.TrimSpace(in.ReceivedDate) != "" {
		d, err := parseDateOnly(in.ReceivedDate)
		if err != nil {
			return nil, err
		}
		receivedAt = d
	}

	items, err := s.buildItems(ctx, order, in.Items)
	if err != nil {
		return nil, err
	}

	docNo, err := s.docNos.Next(ctx, model.DocTypePurchaseReceipt)
	if err != nil {
		return nil, err
	}
	m := &model.PurchaseReceipt{
		DocNo:           docNo,
		PurchaseOrderID: orderID,
		Status:          model.DocumentStatusDraft,
		ReceivedDate:    receivedAt,
		Remark:          strings.TrimSpace(in.Remark),
	}
	if err := s.receipts.CreateWithItems(ctx, m, items); err != nil {
		return nil, err
	}
	s.log.Info("新增采购收货单", "id", m.ID, "doc_no", m.DocNo, "order", order.DocNo, "rows", len(items))
	return s.receipts.FindByID(ctx, m.ID)
}

// buildItems 把入参落成明细，并做两类校验：行属于这张采购单、累计不超订购量。
//
// 超收要连「其余草稿」一起算：只算已确认的话，同一个人可以把同一批货
// 开两张草稿各收一半再一起确认，最终收成两倍。
func (s *purchaseReceiptService) buildItems(
	ctx context.Context, order *model.PurchaseOrder, in []dto.PurchaseReceiptItemInput,
) ([]model.PurchaseReceiptItem, error) {
	byID := make(map[uint]*model.PurchaseOrderItem, len(order.Items))
	for i := range order.Items {
		it := &order.Items[i]
		byID[it.ID] = it
	}

	list, err := s.receipts.ListByOrder(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	received := s.receivedByItem(list)

	out := make([]model.PurchaseReceiptItem, 0, len(in))
	// 同一次提交里可能出现两行指向同一个采购行（不同批次），累加后再判上限。
	thisTime := make(map[uint]money.Decimal, len(in))
	for _, row := range in {
		item, ok := byID[row.PurchaseOrderItemID]
		if !ok {
			return nil, errs.BadRequest("收货明细里有不属于这张采购单的行")
		}
		if !row.Qty.IsPositive() {
			return nil, errs.BadRequest(item.SKUCode + " 的收货数量必须大于 0")
		}
		thisTime[row.PurchaseOrderItemID] = thisTime[row.PurchaseOrderItemID].Add(row.Qty)
		total := received[row.PurchaseOrderItemID].Add(thisTime[row.PurchaseOrderItemID])
		if total.Cmp(item.Qty) > 0 {
			return nil, errs.ErrConflict.WithMsg(
				item.SKUCode + " 累计收货会超过订购量：订购 " + formatQty(item.Qty) +
					"，已收（含草稿）" + formatQty(received[row.PurchaseOrderItemID]) +
					"，本次 " + formatQty(row.Qty))
		}
		out = append(out, model.PurchaseReceiptItem{
			PurchaseOrderItemID: item.ID,
			ProductID:           item.ProductID,
			SKUCode:             item.SKUCode,
			NameCN:              item.NameCN,
			Unit:                item.Unit,
			Qty:                 row.Qty,
			Remark:              strings.TrimSpace(row.Remark),
		})
	}
	return out, nil
}

func (s *purchaseReceiptService) Confirm(
	ctx context.Context, id uint,
) (*model.PurchaseReceipt, error) {
	m, err := s.receipts.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.Status != model.DocumentStatusDraft {
		return nil, errs.ErrConflict.WithMsg("只有草稿状态的收货单可以确认")
	}
	order, err := s.orders.FindByID(ctx, m.PurchaseOrderID)
	if err != nil {
		return nil, err
	}

	issueItems := make([]dto.PurchaseInIssueItem, 0, len(m.Items))
	for _, it := range m.Items {
		issueItems = append(issueItems, dto.PurchaseInIssueItem{
			ProductID: it.ProductID,
			SKUCode:   it.SKUCode,
			NameCN:    it.NameCN,
			Unit:      it.Unit,
			Qty:       it.Qty,
			Remark:    it.Remark,
		})
	}

	partnerID := order.SupplierID
err = s.txs.Transaction(ctx, func(ctx context.Context) error {
		if err := s.receipts.UpdateFields(ctx, id, map[string]any{
			"status": model.DocumentStatusConfirmed,
		}); err != nil {
			return err
		}
		_, err := s.stockIn.IssuePurchaseIn(ctx, dto.PurchaseInIssueInput{
			ReceiptNo: m.DocNo,
			PartnerID: &partnerID,
			DocDate:   m.ReceivedDate,
			Remark:    "采购收货 " + m.DocNo,
			Items:     issueItems,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.log.Info("确认采购收货单", "id", m.ID, "doc_no", m.DocNo, "order", order.DocNo)
	return s.receipts.FindByID(ctx, id)
}

func (s *purchaseReceiptService) Delete(ctx context.Context, id uint) error {
	m, err := s.receipts.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !m.IsEditable() {
		return errs.ErrConflict.WithMsg("已确认的收货单不能删除；它已经动过库存")
	}
	return s.receipts.DeleteByID(ctx, id)
}

// receivedByItem 汇总每行已收量。
//
// 已取消的不算，草稿算：草稿虽然还没入库，但它已经承诺了这批货到，
// 不占量的话同一个人可以开两张草稿各收一半。
func (s *purchaseReceiptService) receivedByItem(
	list []model.PurchaseReceipt,
) map[uint]money.Decimal {
	out := make(map[uint]money.Decimal, len(list))
	for _, r := range list {
		if r.Status == model.DocumentStatusCancelled {
			continue
		}
		for _, it := range r.Items {
			out[it.PurchaseOrderItemID] = out[it.PurchaseOrderItemID].Add(it.Qty)
		}
	}
	return out
}
