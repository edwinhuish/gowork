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
	"github.com/edwinhuish/gowork/internal/repository"
)

// ProductSourceService 管产品货源：一个产品对多个供应商的货号与采购价。
//
// 它是报价单成本的唯一来源。成本之所以不能留在产品表的 cost_price 上：
// 那一个数回答不了"哪家、什么时候、订多少"这三个问题，
// 而比价、备选、旺季分单全靠这三个问题，报价时手填成本就是毛利失真的源头。
type ProductSourceService interface {
	Create(ctx context.Context, in dto.ProductSourceInput) (*model.ProductSource, error)
	Update(ctx context.Context, id uint, in dto.ProductSourceInput) (*model.ProductSource, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.ProductSource, error)
	List(ctx context.Context, q dto.ProductSourceQuery) (*dto.PageResult[model.ProductSource], error)
}

type productSourceService struct {
	repo       repository.ProductSourceRepository
	products   repository.ProductRepository
	partners   repository.PartnerRepository
	currencies CurrencyService
	clock      func() time.Time
	log        *slog.Logger
}

type ProductSourceOption func(*productSourceService)

func WithProductSourceLogger(l *slog.Logger) ProductSourceOption {
	return func(s *productSourceService) { s.log = l }
}

// WithProductSourceClock 注入时钟：采购价按录入当日汇率折本币，测试要能指定"今天"。
func WithProductSourceClock(now func() time.Time) ProductSourceOption {
	return func(s *productSourceService) { s.clock = now }
}

func NewProductSourceService(
	repo repository.ProductSourceRepository,
	products repository.ProductRepository,
	partners repository.PartnerRepository,
	currencies CurrencyService,
	opts ...ProductSourceOption,
) ProductSourceService {
	s := &productSourceService{
		repo:       repo,
		products:   products,
		partners:   partners,
		currencies: currencies,
		clock:      time.Now,
		log:        slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *productSourceService) Create(
	ctx context.Context, in dto.ProductSourceInput,
) (*model.ProductSource, error) {
	if _, err := s.products.FindByID(ctx, in.ProductID); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.BadRequest("产品不存在")
		}
		return nil, err
	}
	if err := ensureSupplierRole(ctx, s.partners, &in.PartnerID); err != nil {
		return nil, err
	}

	terms, err := s.buildTerms(ctx, in, in.ProductID, in.PartnerID, 0)
	if err != nil {
		return nil, err
	}

	m := &model.ProductSource{
		ProductID:         in.ProductID,
		PartnerID:         in.PartnerID,
		SupplierSKU:       terms.SupplierSKU,
		PurchasePrice:     terms.PurchasePrice,
		CurrencyCode:      terms.CurrencyCode,
		PurchasePriceBase: terms.PurchasePriceBase,
		ExchangeRate:      terms.ExchangeRate,
		MOQ:               terms.MOQ,
		DeliveryDays:      terms.DeliveryDays,
		Status:            terms.Status,
		Remark:            terms.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增产品货源", "id", m.ID, "product_id", m.ProductID,
		"partner_id", m.PartnerID, "sku", m.SupplierSKU, "price_base", m.PurchasePriceBase)
	return s.repo.FindByID(ctx, m.ID)
}

func (s *productSourceService) Update(
	ctx context.Context, id uint, in dto.ProductSourceInput,
) (*model.ProductSource, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 换产品等于这条货源描述的是另一个 SKU 的采购；换供应商则连价格都不再对应。
	// 两种都让人新建一条，旧的那条留着，历史报价锁定的成本快照才解释得清。
	if in.ProductID != 0 && in.ProductID != existing.ProductID {
		return nil, errs.BadRequest("货源不能换到另一个产品，请在该产品下新建")
	}
	if in.PartnerID != 0 && in.PartnerID != existing.PartnerID {
		return nil, errs.BadRequest("货源不能换到另一个供应商，请新建一条并从旧货源停用")
	}

	terms, err := s.buildTerms(ctx, in, existing.ProductID, existing.PartnerID, id)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{
		"supplier_sku":        terms.SupplierSKU,
		"purchase_price":      terms.PurchasePrice,
		"currency_code":       terms.CurrencyCode,
		"purchase_price_base": terms.PurchasePriceBase,
		"exchange_rate":       terms.ExchangeRate,
		"moq":                 terms.MOQ,
		"delivery_days":       terms.DeliveryDays,
		"status":              terms.Status,
		"remark":              terms.Remark,
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// Delete 走软删除：历史报价锁定的是成本快照，删掉货源不影响已发生的单据。
// 但没锁进单据的货源一旦被删，报价时就少一个可比价的选择，所以界面上要确认。
func (s *productSourceService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *productSourceService) Get(ctx context.Context, id uint) (*model.ProductSource, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *productSourceService) List(
	ctx context.Context, q dto.ProductSourceQuery,
) (*dto.PageResult[model.ProductSource], error) {
	q.PageQuery.Normalize()

	filter := repository.ProductSourceFilter{
		ProductID: q.ProductID,
		PartnerID: q.PartnerID,
		Keyword:   q.Keyword,
		Status:    enableStatusPtr(q.Status),
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// productSourceTerms 是校验并归一化之后的货源字段。
//
// 与商机同理用结构体而不是 map：新建路径要按字段逐个赋值，
// map 得靠类型断言取出来，断错了是运行时 panic。
type productSourceTerms struct {
	SupplierSKU       string
	PurchasePrice     float64
	CurrencyCode      string
	PurchasePriceBase float64
	ExchangeRate      float64
	MOQ               int
	DeliveryDays      int
	Status            model.EnableStatus
	Remark            string
}

// buildTerms 校验并折算。excludeID 供编辑时把自己排除在查重之外。
func (s *productSourceService) buildTerms(
	ctx context.Context, in dto.ProductSourceInput,
	productID, partnerID, excludeID uint,
) (productSourceTerms, error) {
	var out productSourceTerms

	if in.PurchasePrice < 0 {
		return out, errs.BadRequest("采购价不能为负")
	}
	if in.MOQ < 0 {
		return out, errs.BadRequest("最小起订量不能为负")
	}
	if in.DeliveryDays < 0 || in.DeliveryDays > 3650 {
		return out, errs.BadRequest("交期天数应在 0 到 3650 之间")
	}
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return out, err
	}

	// 按「产品 + 供应商 + 货号」查重：同一家给同一个货号报两次价，
	// 报价时会看到两条价格打架的记录。货号为空也参与查重——
	// 同一产品同一供应商挂两条空货号的行，就是同一件事录了两次。
	sku := strings.TrimSpace(in.SupplierSKU)
	dup, err := s.repo.ExistsBySupplierSKU(ctx, productID, partnerID, sku, excludeID)
	if err != nil {
		return out, err
	}
	if dup {
		if sku == "" {
			return out, errs.ErrConflict.WithMsg("该供应商下已有一条未填货号的货源，请直接编辑那条")
		}
		return out, errs.ErrConflict.WithMsg("该供应商下已有相同货号：" + sku)
	}

	// 币种留空即本币。折算走 CurrencyService：缺当日汇率时给 400，
	// 而不是把 0 写进本币价——那会让最便宜的那条排在第一位。
	baseCode, err := s.currencies.BaseCode(ctx)
	if err != nil {
		return out, err
	}
	code := strings.ToUpper(strings.TrimSpace(in.CurrencyCode))
	if code == "" {
		code = baseCode
	}
	priceBase, rate, err := s.currencies.ToBase(ctx, in.PurchasePrice, code, s.clock())
	if err != nil {
		return out, err
	}

	out = productSourceTerms{
		SupplierSKU:       sku,
		PurchasePrice:     in.PurchasePrice,
		CurrencyCode:      code,
		PurchasePriceBase: priceBase,
		ExchangeRate:      rate,
		MOQ:               in.MOQ,
		DeliveryDays:      in.DeliveryDays,
		Status:            status,
		Remark:            in.Remark,
	}
	return out, nil
}

// ensureSupplierRole 确认 id 指向的主体存在且具备供应商角色。
//
// 不校验启用状态：供应商停用后，已经挂着它的货源仍要能改价、停用。
// 停用的主体在供应商下拉里已经选不到，新引用进不来，这就够了。
func ensureSupplierRole(
	ctx context.Context, partners repository.PartnerRepository, id *uint,
) error {
	if id == nil || *id == 0 {
		return nil
	}
	p, err := partners.FindByID(ctx, *id)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return errs.BadRequest("指定的供应商不存在")
		}
		return err
	}
	if !p.Role.Has(model.PartnerRoleSupplier) {
		return errs.BadRequest(fmt.Sprintf("所选主体不具备供应商角色：%s", p.Name))
	}
	return nil
}
