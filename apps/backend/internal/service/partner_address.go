package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/pkg/errs"
	"github.com/edwinhuish/gowork/internal/repository"
)

// PartnerAddressService 管理主体上的多地址。
//
// 地址是单证取数的依据（发票抬头 / 收货人 / 通知方），所以这一层的职责是
// **保证"每个用途取到哪一条"是确定的**：类型白名单、每条最多一个默认、
// 有地址就必须有默认。取址逻辑本身在 PDF 那一层（T-49 第三步）。
type PartnerAddressService interface {
	List(ctx context.Context, partnerID uint, q dto.PartnerAddressQuery) ([]model.PartnerAddress, error)
	Create(ctx context.Context, partnerID uint, in dto.PartnerAddressInput) (*model.PartnerAddress, error)
	Update(ctx context.Context, partnerID, id uint, in dto.PartnerAddressInput) (*model.PartnerAddress, error)
	Delete(ctx context.Context, partnerID, id uint) error
}

type partnerAddressService struct {
	repo     repository.PartnerAddressRepository
	partners repository.PartnerRepository
	log      *slog.Logger
}

// PartnerAddressOption 供 bootstrap 装配时选择日志器。
type PartnerAddressOption func(*partnerAddressService)

func WithPartnerAddressLogger(log *slog.Logger) PartnerAddressOption {
	return func(s *partnerAddressService) { s.log = log }
}

func NewPartnerAddressService(
	repo repository.PartnerAddressRepository,
	partners repository.PartnerRepository,
	opts ...PartnerAddressOption,
) PartnerAddressService {
	s := &partnerAddressService{repo: repo, partners: partners, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *partnerAddressService) List(
	ctx context.Context, partnerID uint, q dto.PartnerAddressQuery,
) ([]model.PartnerAddress, error) {
	if _, err := s.partners.FindByID(ctx, partnerID); err != nil {
		return nil, err
	}
	var typ model.PartnerAddressType
	if q.Type != "" {
		t, err := normAddressType(q.Type)
		if err != nil {
			return nil, err
		}
		typ = t
	}
	return s.repo.ListByPartner(ctx, partnerID, typ)
}

func (s *partnerAddressService) Create(
	ctx context.Context, partnerID uint, in dto.PartnerAddressInput,
) (*model.PartnerAddress, error) {
	if _, err := s.partners.FindByID(ctx, partnerID); err != nil {
		return nil, err
	}
	typ, err := normAddressType(in.Type)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Address) == "" {
		return nil, errs.BadRequest("地址不能为空")
	}

	m := &model.PartnerAddress{
		PartnerID:  partnerID,
		Type:       typ,
		IsDefault:  in.IsDefault,
		Contact:    strings.TrimSpace(in.Contact),
		Phone:      strings.TrimSpace(in.Phone),
		Country:    strings.TrimSpace(in.Country),
		Province:   strings.TrimSpace(in.Province),
		City:       strings.TrimSpace(in.City),
		PostalCode: strings.TrimSpace(in.PostalCode),
		Address:    strings.TrimSpace(in.Address),
		Remark:     strings.TrimSpace(in.Remark),
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("新增主体地址", "partner", partnerID, "type", typ, "default", m.IsDefault)
	return m, nil
}

func (s *partnerAddressService) Update(
	ctx context.Context, partnerID, id uint, in dto.PartnerAddressInput,
) (*model.PartnerAddress, error) {
	current, err := s.loadOwned(ctx, partnerID, id)
	if err != nil {
		return nil, err
	}
	typ, err := normAddressType(in.Type)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Address) == "" {
		return nil, errs.BadRequest("地址不能为空")
	}

	fields := map[string]any{
		"type":        typ,
		"is_default":  in.IsDefault,
		"contact":     strings.TrimSpace(in.Contact),
		"phone":       strings.TrimSpace(in.Phone),
		"country":     strings.TrimSpace(in.Country),
		"province":    strings.TrimSpace(in.Province),
		"city":        strings.TrimSpace(in.City),
		"postal_code": strings.TrimSpace(in.PostalCode),
		"address":     strings.TrimSpace(in.Address),
		"remark":      strings.TrimSpace(in.Remark),
	}
	if err := s.repo.Update(ctx, current, fields); err != nil {
		return nil, err
	}
	s.log.Info("更新主体地址", "partner", partnerID, "id", id, "type", typ)
	return s.repo.FindByID(ctx, id)
}

func (s *partnerAddressService) Delete(ctx context.Context, partnerID, id uint) error {
	current, err := s.loadOwned(ctx, partnerID, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, current); err != nil {
		return err
	}
	s.log.Info("删除主体地址", "partner", partnerID, "id", id)
	return nil
}

// loadOwned 取地址并确认它属于这条路径上的主体。
//
// 不这么做的话，`PUT /partners/1/addresses/9` 能改到别家的地址——
// id 是全局唯一的，路径上的 partnerID 只是"看起来"限定了范围。
// 越界一律按不存在处理，避免用状态码反推某条地址属于谁。
func (s *partnerAddressService) loadOwned(
	ctx context.Context, partnerID, id uint,
) (*model.PartnerAddress, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil, errs.ErrNotFound
		}
		return nil, err
	}
	if m.PartnerID != partnerID {
		return nil, errs.ErrNotFound
	}
	return m, nil
}

// normAddressType 校验用途。失败时把可选值列出来——
// 这个错误几乎只会在前端拼错字段时出现，值得直接告诉人该填什么。
func normAddressType(raw string) (model.PartnerAddressType, error) {
	t := model.PartnerAddressType(strings.ToLower(strings.TrimSpace(raw)))
	if !t.IsValid() {
		return "", errs.BadRequest("地址用途只能是 invoice / delivery / consignee / notify")
	}
	return t, nil
}
