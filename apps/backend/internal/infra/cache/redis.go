package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/edwinhuish/gowork/internal/config"
)

type redisCache struct {
	client *redis.Client
	prefix string
}

func newRedis(cfg config.Redis) *redisCache {
	return &redisCache{
		client: redis.NewClient(&redis.Options{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
		prefix: cfg.Prefix,
	}
}

func (c *redisCache) k(key string) string { return c.prefix + key }

func (c *redisCache) Get(ctx context.Context, key string) (string, error) {
	v, err := c.client.Get(ctx, c.k(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrMiss
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func (c *redisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.client.Set(ctx, c.k(key), value, ttl).Err()
}

func (c *redisCache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = c.k(k)
	}
	return c.client.Del(ctx, prefixed...).Err()
}

// Incr 用 pipeline 保证"自增 + 首次设置过期"在同一个往返里完成。
func (c *redisCache) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	fullKey := c.k(key)
	pipe := c.client.TxPipeline()
	incr := pipe.Incr(ctx, fullKey)
	pipe.Expire(ctx, fullKey, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (c *redisCache) Ping(ctx context.Context) error { return c.client.Ping(ctx).Err() }

func (c *redisCache) Driver() string { return "redis" }

func (c *redisCache) Close() error { return c.client.Close() }
