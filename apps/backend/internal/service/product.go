package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	codenorm "github.com/edwinhuish/gowork/internal/pkg/code"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

type ProductService interface {
	Create(ctx context.Context, in dto.ProductInput) (*model.Product, error)
	Update(ctx context.Context, id uint, in dto.ProductInput) (*model.Product, error)
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*model.Product, error)
	List(ctx context.Context, q dto.ProductQuery) (*dto.PageResult[model.Product], error)
	// Restore 把已软删的产品恢复回来。删除是可追溯的，恢复就该有入口，
	// 否则「删错了」只能靠改库。
	Restore(ctx context.Context, id uint) (*model.Product, error)
}

type productService struct {
	repo repository.ProductRepository
	// partners 用来校验 supplier_id 指向的主体确实具备供应商角色。
	// 合并后的主表什么角色都可能，光有外键约束不住"选了个客户当供应商"。
	partners repository.PartnerRepository
	// sources 只在删除前判断有没有货源挂着这个产品。
	// 产品的 supplier_id / cost_price 是默认货源的冗余，真货源在 md_product_source。
	sources repository.ProductSourceRepository
	// codes 是产品的别名编号（旧 SKU、客户号、供应商货号）：
	// 改 SKU 时要沉淀旧码，新建与改 SKU 时要拦住「SKU 撞上别人用过的编号」。
	codes repository.ProductCodeRepository
	// txs 让「清编号 + 删产品」落在同一个事务里：只删产品会留下孤儿编号，
	// 它仍会被「新 SKU 不得等于别人用过的编号」算进去。
	txs repository.TxManager
	log *slog.Logger
}

// ProductOption 用于注入可选依赖，保持构造函数签名稳定。
type ProductOption func(*productService)

func WithProductLogger(l *slog.Logger) ProductOption {
	return func(s *productService) { s.log = l }
}

func NewProductService(
	repo repository.ProductRepository,
	partners repository.PartnerRepository,
	sources repository.ProductSourceRepository,
	codes repository.ProductCodeRepository,
	txs repository.TxManager,
	opts ...ProductOption,
) ProductService {
	s := &productService{
		repo: repo, partners: partners, sources: sources, codes: codes, txs: txs,
		log: slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *productService) Create(ctx context.Context, in dto.ProductInput) (*model.Product, error) {
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}

	dup, err := s.repo.ExistsBySKUCode(ctx, in.SKUCode, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("产品编码已存在：" + in.SKUCode)
	}
	if err := s.checkSKUNotUsedAsCode(ctx, in.SKUCode, 0); err != nil {
		return nil, err
	}
	if err := s.checkSupplier(ctx, in.SupplierID); err != nil {
		return nil, err
	}

	m := &model.Product{
		SKUCode:    in.SKUCode,
		NameCN:     in.NameCN,
		NameEN:     in.NameEN,
		Category:   in.Category,
		Spec:       in.Spec,
		Unit:       defaultUnit(in.Unit),
		SupplierID: in.SupplierID,
		CostPrice:  in.CostPrice,
		SalePrice:  in.SalePrice,

		HSCode:         in.HSCode,
		DeclaredNameCN: in.DeclaredNameCN,
		DeclaredNameEN: in.DeclaredNameEN,
		Material:       in.Material,
		OriginCountry:  in.OriginCountry,
		LegalUnit:      in.LegalUnit,

		Status: status,
		Remark: in.Remark,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}

	s.log.Info("新增产品", "id", m.ID, "sku", m.SKUCode)
	return m, nil
}

func (s *productService) Update(ctx context.Context, id uint, in dto.ProductInput) (*model.Product, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	status, err := normEnableStatus(in.Status, model.EnableStatusOn)
	if err != nil {
		return nil, err
	}

	dup, err := s.repo.ExistsBySKUCode(ctx, in.SKUCode, id)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg("产品编码已存在：" + in.SKUCode)
	}
	skuChanged := existing.SKUCode != in.SKUCode
	if skuChanged {
		if err := s.checkSKUNotUsedAsCode(ctx, in.SKUCode, id); err != nil {
			return nil, err
		}
	}
	if err := s.checkSupplier(ctx, in.SupplierID); err != nil {
		return nil, err
	}

	fields := map[string]any{
		"sku_code":    in.SKUCode,
		"name_cn":     in.NameCN,
		"name_en":     in.NameEN,
		"category":    in.Category,
		"spec":        in.Spec,
		"unit":        defaultUnit(in.Unit),
		"supplier_id": in.SupplierID,
		"cost_price":  in.CostPrice,
		"sale_price":  in.SalePrice,

		"hs_code":          in.HSCode,
		"declared_name_cn": in.DeclaredNameCN,
		"declared_name_en": in.DeclaredNameEN,
		"material":         in.Material,
		"origin_country":   in.OriginCountry,
		"legal_unit":       in.LegalUnit,

		"status": status,
		"remark": in.Remark,
	}
	if in.UpdatedAt != nil {
		fields[repository.FieldUpdatedAt] = *in.UpdatedAt
	}
	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	if skuChanged {
		// 归档放在更新之后：先改成功再说。反过来的话，更新失败时旧 SKU 已经在
		// 编号表里躺着，列表上会出现「命中编号 ABC」而 SKU 也还是 ABC 的重复提示。
		// 两步目前不在同一个事务里，等跨仓储事务边界（T-42）落地后再合并。
		if err := s.archiveOldSKU(ctx, id, existing.SKUCode); err != nil {
			return nil, err
		}
	}
	return s.repo.FindByID(ctx, id)
}

// Delete 在有货源挂靠时拒绝删除。
//
// 产品的软删除不会带走 md_product_source 里的行，那些行会在"按供应商查货源"
// 的列表里继续出现，而按产品查又查不到——典型的表现是某家供应商的价比对
// 突然多出一条点不进去的记录。让人先删货源，比事后清理孤儿行便宜。
func (s *productService) Delete(ctx context.Context, id uint) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	n, err := s.sources.CountByProductID(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errs.ErrConflict.WithMsg(
			fmt.Sprintf("该产品还有 %d 条货源记录，请先在产品详情页删除货源", n))
	}
	// 编号必须跟着产品一起消失。产品是软删，编号行却不在软删范围内：
	// 留下的孤儿行没有任何入口能删掉（编号面板按 product_id 查，产品已不在了），
	// 却会让「新 SKU 不得等于任何产品用过的编号」永久拒绝那个号。
	return s.txs.Transaction(ctx, func(ctx context.Context) error {
		if err := s.codes.DeleteByProductID(ctx, id); err != nil {
			return err
		}
		return s.repo.Delete(ctx, id)
	})
}

func (s *productService) Get(ctx context.Context, id uint) (*model.Product, error) {
	return s.repo.FindByID(ctx, id)
}

// Restore 把一条已软删的产品恢复回来。
//
// 只把标记写回 0，不会把删除时清掉的编号一并恢复：编号是"别人怎么叫这件货"，
// 产品下线那段时间里客户可能已经把那个号挪给了别的货，静默恢复等于替用户断言
// "这个号还属于你"。需要的话在产品详情页重新录。
func (s *productService) Restore(ctx context.Context, id uint) (*model.Product, error) {
	m, err := s.repo.FindByIDIncludingDeleted(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.DeletedAt == 0 {
		return nil, errs.BadRequest("该产品未被删除，无需恢复")
	}

	// 先挡一次编码冲突。数据库唯一索引也能拦住，但那时的提示是通用的
	// 「数据已存在」，用户看不出是哪个号被占了、也看不出下一步该做什么。
	occupied, err := s.repo.ExistsBySKUCode(ctx, m.SKUCode, id)
	if err != nil {
		return nil, err
	}
	if occupied {
		return nil, errs.ErrConflict.WithMsg(fmt.Sprintf(
			"SKU %s 已被另一条产品占用，先处理掉那条再恢复", m.SKUCode))
	}

	if err := s.repo.Restore(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

func (s *productService) List(ctx context.Context, q dto.ProductQuery) (*dto.PageResult[model.Product], error) {
	q.PageQuery.Normalize()

	filter := repository.ProductFilter{
		Keyword:        q.Keyword,
		Category:       q.Category,
		SupplierID:     q.SupplierID,
		Status:         enableStatusPtr(q.Status),
		IncludeDeleted: q.IncludeDeleted,
	}
	page := repository.Pagination{Page: q.Page, PageSize: q.PageSize}

	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, err
	}

	// 命中编号只在这一页上回填：搜旧 SKU 时能看到"原来是这个号"，
	// 否则用户只知道这条出现了，不知道为什么。
	if q.Keyword != "" && len(list) > 0 {
		ids := make([]uint, 0, len(list))
		for _, p := range list {
			ids = append(ids, p.ID)
		}
		hits, err := s.codes.MatchByKeyword(ctx, ids, q.Keyword)
		if err != nil {
			return nil, err
		}
		// 供应商货号在货源表上，同属"别人怎么叫这件货"，一起回填；
		// 漏了它就会出现"按货号搜到了，但看不出是这个号"。
		sourceHits, err := s.sources.MatchSupplierSKUByKeyword(ctx, ids, q.Keyword)
		if err != nil {
			return nil, err
		}
		for i := range list {
			list[i].MatchedCodes = mergeCodes(hits[list[i].ID], sourceHits[list[i].ID])
		}
	}
	return dto.NewPageResult(list, total, q.Page, q.PageSize), nil
}

// checkSKUNotUsedAsCode 拦住「新 SKU 等于某个产品用过的编号」。
//
// 编号会被搜到，SKU 也会被搜到：两者同值时，搜索结果里会出现两条看上去一样的货，
// 用户没法判断该点哪一条。excludeProductID 排除自己——自己的旧码不是冲突。
func (s *productService) checkSKUNotUsedAsCode(
	ctx context.Context, sku string, excludeProductID uint,
) error {
	norm := codenorm.Normalize(sku)
	if norm == "" {
		return nil
	}
	dup, err := s.codes.ExistsNormAnywhere(ctx, norm, excludeProductID)
	if err != nil {
		return err
	}
	if dup {
		return errs.ErrConflict.WithMsg(
			"这个编码已被用作其他产品的编号（旧 SKU / 客户号 / 供应商货号），请换一个 SKU")
	}
	return nil
}

// archiveOldSKU 把用过的 SKU 沉淀成一条编号。
//
// 客户手里还拿着旧 SKU 的报价单与邮件，改完 SKU 之后按旧号搜不到货，
// 这单在系统里就再也对不上了。旧码只增不改，SKU 可以继续改。
func (s *productService) archiveOldSKU(ctx context.Context, productID uint, oldSKU string) error {
	norm := codenorm.Normalize(oldSKU)
	if norm == "" {
		return nil
	}
	exists, err := s.codes.ExistsNorm(ctx, productID, norm, 0)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.codes.Create(ctx, &model.ProductCode{
		ProductID: productID,
		Code:      oldSKU,
		CodeNorm:  norm,
		Source:    model.ProductCodeSourceSKU,
	})
}

// mergeCodes 把编号表与货源表的命中合并成一条提示，去重时按归一化值判断：
// 同一个号可能既录进了编号表、又挂在货源上，两处写法不同也算同一个号，
// 提示里出现两次会让用户以为有两条不同的编号。
func mergeCodes(fromCodes, fromSources []string) []string {
	if len(fromSources) == 0 {
		return fromCodes
	}
	out := make([]string, 0, len(fromCodes)+len(fromSources))
	seen := make(map[string]bool, cap(out))
	for _, group := range [][]string{fromCodes, fromSources} {
		for _, c := range group {
			key := codenorm.Normalize(c)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, c)
		}
	}
	return out
}

func defaultUnit(unit string) string {
	if unit == "" {
		return "PCS"
	}
	return unit
}

// checkSupplier 确认 supplier_id 指向的主体存在且具备供应商角色。
//
// 规则本身在 ensureSupplierRole 里，货源模块用的是同一份：产品上的默认货源
// 与 md_product_source 里的货源行对"供应商"的定义必须一致，各写一份迟早分叉。
func (s *productService) checkSupplier(ctx context.Context, id *uint) error {
	return ensureSupplierRole(ctx, s.partners, id)
}
