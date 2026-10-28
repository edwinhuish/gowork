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
	products  repository.ProductRepository
	suppliers repository.SupplierRepository
	customers repository.CustomerRepository
	cache     cache.Cache
	log       *slog.Logger
}

type DashboardOption func(*dashboardService)

func WithDashboardLogger(l *slog.Logger) DashboardOption {
	return func(s *dashboardService) { s.log = l }
}

func NewDashboardService(
	products repository.ProductRepository,
	suppliers repository.SupplierRepository,
	customers repository.CustomerRepository,
	c cache.Cache,
	opts ...DashboardOption,
) DashboardService {
	s := &dashboardService{
		products:  products,
		suppliers: suppliers,
		customers: customers,
		cache:     c,
		log:       slog.Default(),
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
	if summary.ActiveProducts, err = s.products.CountByEnableStatus(ctx, model.EnableStatusOn); err != nil {
		return nil, err
	}
	if summary.SupplierTotal, err = s.suppliers.Count(ctx); err != nil {
		return nil, err
	}
	if summary.CustomerTotal, err = s.customers.Count(ctx); err != nil {
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
