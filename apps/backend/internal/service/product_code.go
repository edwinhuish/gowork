package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	codenorm "github.com/edwinhuish/gowork/internal/pkg/code"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// productCodeSources 是允许的编号来源。
//
// 用白名单而不是自由文本：来源决定"这个号是谁叫的"，写成「客户」「客户A」
// 「customer」三种写法之后，就再也统计不出"有多少号是客户给的"。
var productCodeSources = map[string]bool{
	model.ProductCodeSourceSKU:      true,
	model.ProductCodeSourceCustomer: true,
	model.ProductCodeSourceSupplier: true,
	model.ProductCodeSourceFactory:  true,
	model.ProductCodeSourceOther:    true,
}

// ProductCodeService 管产品的别名编号：客户物料号、供应商货号、工厂号、旧 SKU。
//
// 它只负责"录得对、搜得到"；能不能被搜到由产品列表的关键词条件决定，
// 两处共用 pkg/code 的归一化函数，口径不会分叉。
type ProductCodeService interface {
	List(ctx context.Context, q dto.ProductCodeQuery) ([]model.ProductCode, error)
	Create(ctx context.Context, in dto.ProductCodeInput) (*model.ProductCode, error)
	Delete(ctx context.Context, id uint) error
}

type productCodeService struct {
	codes    repository.ProductCodeRepository
	products repository.ProductRepository
	log      *slog.Logger
}

type ProductCodeOption func(*productCodeService)

func WithProductCodeLogger(l *slog.Logger) ProductCodeOption {
	return func(s *productCodeService) { s.log = l }
}

func NewProductCodeService(
	codes repository.ProductCodeRepository,
	products repository.ProductRepository,
	opts ...ProductCodeOption,
) ProductCodeService {
	s := &productCodeService{codes: codes, products: products, log: slog.Default()}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *productCodeService) List(
	ctx context.Context, q dto.ProductCodeQuery,
) ([]model.ProductCode, error) {
	if _, err := s.products.FindByID(ctx, q.ProductID); err != nil {
		return nil, err
	}
	return s.codes.ListByProductID(ctx, q.ProductID)
}

func (s *productCodeService) Create(
	ctx context.Context, in dto.ProductCodeInput,
) (*model.ProductCode, error) {
	code := strings.TrimSpace(in.Code)
	norm := codenorm.Normalize(code)
	if norm == "" {
		return nil, errs.BadRequest("编号不能只有分隔符")
	}

	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = model.ProductCodeSourceOther
	}
	if !productCodeSources[source] {
		return nil, errs.BadRequest("编号来源不合法：" + source)
	}

	product, err := s.products.FindByID(ctx, in.ProductID)
	if err != nil {
		return nil, err
	}

	// 同一个编号不能和这个产品自己的 SKU 重名：那会让列表上「命中编号」和
	// 主标识是同一个词，看着像重复数据。
	if norm == codenorm.Normalize(product.SKUCode) {
		return nil, errs.ErrConflict.WithMsg("这个号就是它自己的 SKU，不必再录一遍")
	}

	dup, err := s.codes.ExistsNorm(ctx, in.ProductID, norm, 0)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errs.ErrConflict.WithMsg(
			fmt.Sprintf("该产品已经有编号 %s（大小写与分隔符不区分）", code))
	}

	m := &model.ProductCode{
		ProductID: in.ProductID,
		Code:      code,
		CodeNorm:  norm,
		Source:    source,
		Remark:    strings.TrimSpace(in.Remark),
	}
	if err := s.codes.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增产品编号", "id", m.ID, "product_id", m.ProductID,
		"code", m.Code, "source", m.Source)
	return m, nil
}

func (s *productCodeService) Delete(ctx context.Context, id uint) error {
	if err := s.codes.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("删除产品编号", "id", id)
	return nil
}
