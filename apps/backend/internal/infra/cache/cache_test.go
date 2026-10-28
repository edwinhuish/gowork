package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/edwinhuish/gowork/internal/config"
)

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func ptrBool(b bool) *bool { return &b }

// newTestMemory 构造一个内存缓存，默认不落盘（除持久化用例）。
func newTestMemory(t *testing.T, mem config.MemoryConfig) *memoryCache {
	t.Helper()

	c, err := newMemory(config.Cache{
		Driver: config.CacheMemory,
		Prefix: "test:",
		Memory: mem,
	}, testLogger())
	if err != nil {
		t.Fatalf("构造内存缓存失败: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestCacheContract 用同一套断言验证两套实现，确保可以按环境互换。
// 设置 GOWORK_TEST_REDIS_ADDR 才会跑 redis 分支。
func TestCacheContract(t *testing.T) {
	ctx := context.Background()

	impls := map[string]Cache{
		"memory": newTestMemory(t, config.MemoryConfig{Persist: ptrBool(false)}),
	}

	if addr := os.Getenv("GOWORK_TEST_REDIS_ADDR"); addr != "" {
		c, err := New(config.Cache{
			Driver: config.CacheRedis,
			// 每次跑用独立前缀并落在 15 号库，避免污染应用数据。
			Prefix: fmt.Sprintf("ct:%d:", time.Now().UnixNano()),
			Redis:  config.RedisConfig{Addr: addr, DB: 15, DialTimeout: 2},
		}, testLogger())
		if err != nil {
			t.Skipf("redis 不可用，跳过: %v", err)
		}
		t.Cleanup(func() { _ = c.Close() })
		impls["redis"] = c
	}

	for name, c := range impls {
		t.Run(name, func(t *testing.T) {
			t.Run("未命中返回 ErrMiss", func(t *testing.T) {
				if _, err := c.Get(ctx, "absent"); !errors.Is(err, ErrMiss) {
					t.Fatalf("期望 ErrMiss，得到 %v", err)
				}
			})

			t.Run("写入后可读回", func(t *testing.T) {
				if err := c.Set(ctx, "k1", "v1", time.Minute); err != nil {
					t.Fatalf("Set: %v", err)
				}
				v, err := c.Get(ctx, "k1")
				if err != nil || v != "v1" {
					t.Fatalf("Get = %q, %v", v, err)
				}
			})

			t.Run("ttl 到期即消失", func(t *testing.T) {
				if err := c.Set(ctx, "k2", "v2", 40*time.Millisecond); err != nil {
					t.Fatalf("Set: %v", err)
				}
				time.Sleep(90 * time.Millisecond)
				if _, err := c.Get(ctx, "k2"); !errors.Is(err, ErrMiss) {
					t.Fatalf("过期键应返回 ErrMiss，得到 %v", err)
				}
			})

			t.Run("ttl<=0 表示永不过期", func(t *testing.T) {
				if err := c.Set(ctx, "k3", "v3", 0); err != nil {
					t.Fatalf("Set: %v", err)
				}
				time.Sleep(30 * time.Millisecond)
				if v, err := c.Get(ctx, "k3"); err != nil || v != "v3" {
					t.Fatalf("零 TTL 应永久有效，得到 %q, %v", v, err)
				}
			})

			t.Run("Del 后可删除多个键", func(t *testing.T) {
				_ = c.Set(ctx, "d1", "1", time.Minute)
				_ = c.Set(ctx, "d2", "2", time.Minute)
				if err := c.Del(ctx, "d1", "d2"); err != nil {
					t.Fatalf("Del: %v", err)
				}
				for _, k := range []string{"d1", "d2"} {
					if _, err := c.Get(ctx, k); !errors.Is(err, ErrMiss) {
						t.Fatalf("%s 应已删除，得到 %v", k, err)
					}
				}
				if err := c.Del(ctx); err != nil {
					t.Fatalf("空参数 Del 应无操作: %v", err)
				}
			})

			t.Run("Incr 自增并刷新窗口", func(t *testing.T) {
				key := "cnt"
				for want := int64(1); want <= 2; want++ {
					got, err := c.Incr(ctx, key, 150*time.Millisecond)
					if err != nil || got != want {
						t.Fatalf("第 %d 次 Incr = %d, %v", want, got, err)
					}
					time.Sleep(80 * time.Millisecond)
				}
				// 每次自增都重置 TTL，此刻距上次自增仅 80ms，还没过期。
				if _, err := c.Get(ctx, key); err != nil {
					t.Fatalf("滑动窗口内不应过期: %v", err)
				}
				time.Sleep(200 * time.Millisecond)
				if _, err := c.Get(ctx, key); !errors.Is(err, ErrMiss) {
					t.Fatalf("窗口结束后应过期，得到 %v", err)
				}
			})

			t.Run("Incr 撞上非数字值返回 ErrNotInteger", func(t *testing.T) {
				_ = c.Set(ctx, "bad", "abc", time.Minute)
				if _, err := c.Incr(ctx, "bad", time.Minute); !errors.Is(err, ErrNotInteger) {
					t.Fatalf("期望 ErrNotInteger，得到 %v", err)
				}
			})

			t.Run("Ping 与 Driver", func(t *testing.T) {
				if err := c.Ping(ctx); err != nil {
					t.Fatalf("Ping: %v", err)
				}
				if c.Driver() == "" {
					t.Fatal("Driver 不应为空")
				}
			})
		})
	}
}

// TestMemoryPersistsAcrossRestart 验证落盘：正常 Close 后重建实例，未过期数据仍在。
func TestMemoryPersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "cache.snapshot")
	cfg := config.MemoryConfig{Persist: ptrBool(true), Path: path, SaveInterval: 3600}

	first, err := newMemory(config.Cache{Driver: config.CacheMemory, Prefix: "p:", Memory: cfg}, testLogger())
	if err != nil {
		t.Fatalf("第一次启动失败: %v", err)
	}
	if err := first.Set(ctx, "session:1", "42", time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := first.Incr(ctx, "login_fail:admin", time.Hour); err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if err := first.Set(ctx, "expired", "x", 10*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close 应把数据落盘: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("快照文件未生成: %v", err)
	}

	time.Sleep(30 * time.Millisecond) // 让 expired 这个键确认过期

	second, err := newMemory(config.Cache{Driver: config.CacheMemory, Prefix: "p:", Memory: cfg}, testLogger())
	if err != nil {
		t.Fatalf("重启失败: %v", err)
	}
	defer func() { _ = second.Close() }()

	if v, err := second.Get(ctx, "session:1"); err != nil || v != "42" {
		t.Fatalf("会话未恢复: %q, %v", v, err)
	}
	if v, err := second.Get(ctx, "login_fail:admin"); err != nil || v != "1" {
		t.Fatalf("计数未恢复: %q, %v", v, err)
	}
	if _, err := second.Get(ctx, "expired"); !errors.Is(err, ErrMiss) {
		t.Fatalf("已过期的键不应被恢复，得到 %v", err)
	}
}

// TestMemorySurvivesCorruptSnapshot 验证快照损坏不会拖垮启动。
func TestMemorySurvivesCorruptSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.snapshot")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("准备损坏文件: %v", err)
	}

	c := newTestMemory(t, config.MemoryConfig{Persist: ptrBool(true), Path: path, SaveInterval: 3600})

	if _, err := c.Get(context.Background(), "anything"); !errors.Is(err, ErrMiss) {
		t.Fatalf("应以空缓存启动，得到 %v", err)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("损坏文件应被备份: %v", err)
	}
}
