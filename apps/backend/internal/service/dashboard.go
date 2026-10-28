package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/edwinhuish/gowork/internal/dto"
	"github.com/edwinhuish/gowork/internal/infra/cache"
	"github.com/edwinhuish/gowork/internal/model"
	"github.com/edwinhuish/gowork/internal/repository"
)

const dashboardCacheKey = "dashboard:summary"
const dashboardCacheTTL = 30 * time.Second

type DashboardService interface {
	Summary(ctx context.Context) (*dto.DashboardSummary, error)
}

// dashboardService 演示了"聚合多个仓储 + 结果缓存"的典型服务：
// 它不持有任何一个 model 的写权限，只做读取与汇总。
type dashboardService struct {
	products repository.ProductRepository
	partners repository.PartnerRepository
	cache    cache.Cache
	log      *slog.Logger
}

type DashboardOption func(*dashboardService)

func WithDashboardLogger(l *slog.Logger) DashboardOption {
	return func(s *dashboardService) { s.log = l }
}

func NewDashboardService(
	products repository.ProductRepository,
	partners repository.PartnerRepository,
	c cache.Cache,
	opts ...DashboardOption,
) DashboardService {
	s := &dashboardService{
		products: products,
		partners: partners,
		cache:    c,
		log:      slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *dashboardService) Summary(ctx context.Context) (*dto.DashboardSummary, error) {
	if s.cache != nil {
		if raw, err := s.cache.Get(ctx, dashboardCacheKey); err == nil {
			var cached dto.DashboardSummary
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return &cached, nil
			}
		}
	}

	var (
		summary dto.DashboardSummary
		err     error
	)
	if summary.ProductTotal, err = s.products.Count(ctx); err != nil {
		return nil, err
	}
	if summary.EnabledProducts, err = s.products.CountByEnableStatus(ctx, model.EnableStatusOn); err != nil {
		return nil, err
	}
	// 供应商与客户现在是同一张表的两种角色，按角色计数。
	// "客供两用"的主体在两个指标里各计一次 —— 它确实同时是两种身份，
	// 只是共用一条记录、一套联系方式，这正是合并想达到的效果。
	if summary.SupplierTotal, err = s.partners.CountByRole(ctx, model.PartnerRoleSupplier); err != nil {
		return nil, err
	}
	if summary.CustomerTotal, err = s.partners.CountByRole(ctx, model.PartnerRoleCustomer); err != nil {
		return nil, err
	}

	if s.cache != nil {
		if raw, err := json.Marshal(summary); err == nil {
			if err := s.cache.Set(ctx, dashboardCacheKey, string(raw), dashboardCacheTTL); err != nil {
				s.log.Warn("写入首页缓存失败", "err", err)
			}
		}
	}
	return &summary, nil
}
