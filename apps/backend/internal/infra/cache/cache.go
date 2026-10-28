// Package cache 提供统一的缓存抽象。
//
// 上层只依赖 Cache 接口：生产用 Redis，本地开发在 Redis 不可用时自动降级到进程内实现，
// 这样服务在任何机器上都能直接跑起来，同时不改变业务代码。
package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/edwinhuish/gowork/internal/config"
)

// ErrMiss 表示键不存在（区别于"缓存不可用"这类真实错误）。
var ErrMiss = errors.New("cache: key not found")

type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	// Incr 自增并在首次写入时设置 TTL，用于登录失败计数、限流等场景。
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Ping(ctx context.Context) error
	Driver() string
	Close() error
}

// New 创建缓存实例。优先连接 Redis，失败时按配置决定是否降级。
func New(cfg config.Redis, log *slog.Logger) (Cache, error) {
	if cfg.Addr == "" {
		log.Warn("未配置 redis.addr，使用进程内缓存")
		return newMemory(), nil
	}

	rc := newRedis(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rc.Ping(ctx); err == nil {
		log.Info("缓存已连接", "driver", "redis", "addr", cfg.Addr)
		return rc, nil
	} else if !cfg.FallbackToMemory {
		_ = rc.Close()
		return nil, err
	} else {
		log.Warn("redis 不可用，降级为进程内缓存", "addr", cfg.Addr, "err", err)
		_ = rc.Close()
		return newMemory(), nil
	}
}
