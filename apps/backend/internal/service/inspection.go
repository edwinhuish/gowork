package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// InspectionService 管验货记录（T-51b）。
//
// 它补的是「我验过货」这句话的证据：结论、抽检比例、缺陷项与附件照片。
// 在此之前只有跟单节点上的一个完成日期——出了问题，既证明不了验过、
// 也说不清当时验出了什么。
type InspectionService interface {
	List(ctx context.Context, orderID uint) ([]model.Inspection, error)
	Create(ctx context.Context, orderID uint, in dto.InspectionInput) (*model.Inspection, error)
	// Delete 删除一条录错的记录。不做编辑：验货是"某天某人验了这批货"的事实。
	Delete(ctx context.Context, id uint) error
}

type inspectionService struct {
	inspections repository.InspectionRepository
	orders      repository.PurchaseOrderRepository
	log         *slog.Logger
}

type InspectionOption func(*inspectionService)

func WithInspectionLogger(l *slog.Logger) InspectionOption {
	return func(s *inspectionService) { s.log = l }
}

func NewInspectionService(
	inspections repository.InspectionRepository,
	orders repository.PurchaseOrderRepository,
	opts ...InspectionOption,
) InspectionService {
	s := &inspectionService{inspections: inspections, orders: orders, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *inspectionService) List(
	ctx context.Context, orderID uint,
) ([]model.Inspection, error) {
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	rows, err := s.inspections.ListByOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	// 回填 SKU：验货记录列表上只写"合格 / 不合格"看不出验的是哪一批货。
	sku := make(map[uint]string, len(order.Items))
	for _, it := range order.Items {
		sku[it.ID] = it.SKUCode
	}
	for i := range rows {
		if rows[i].PurchaseOrderItemID != nil {
			rows[i].SKUCode = sku[*rows[i].PurchaseOrderItemID]
		}
	}
	return rows, nil
}

func (s *inspectionService) Create(
	ctx context.Context, orderID uint, in dto.InspectionInput,
) (*model.Inspection, error) {
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	result := model.InspectionResult(strings.TrimSpace(in.Result))
	if !result.IsValid() {
		return nil, errs.BadRequest("验货结论只能是 passed / failed / partial")
	}

	// 挂到具体行时校验归属：否则 A 单的验货记录能挂到 B 单的行上，
	// 而列表按订单查又查不出来——等于多出一条谁也解释不清的记录。
	if in.PurchaseOrderItemID != nil {
		found := false
		for _, it := range order.Items {
			if it.ID == *in.PurchaseOrderItemID {
				found = true
				break
			}
		}
		if !found {
			return nil, errs.BadRequest("验货记录挂的采购行不属于这张采购单")
		}
	}

	inspectedAt := model.DateOnly(time.Now())
	if strings.TrimSpace(in.InspectedAt) != "" {
		d, err := parseDateOnly(in.InspectedAt)
		if err != nil {
			return nil, err
		}
		inspectedAt = d
	}

	m := &model.Inspection{
		PurchaseOrderID:     orderID,
		PurchaseOrderItemID: in.PurchaseOrderItemID,
		Result:              result,
		SampleRate:          in.SampleRate,
		InspectedAt:         inspectedAt,
		Inspector:           strings.TrimSpace(in.Inspector),
		Defects:             strings.TrimSpace(in.Defects),
		Remark:              strings.TrimSpace(in.Remark),
	}
	if err := s.inspections.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增验货记录", "id", m.ID, "order", order.DocNo, "result", result)
	return m, nil
}

func (s *inspectionService) Delete(ctx context.Context, id uint) error {
	if _, err := s.inspections.FindByID(ctx, id); err != nil {
		return err
	}
	return s.inspections.DeleteByID(ctx, id)
}
