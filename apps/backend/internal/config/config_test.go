package config

import (
	"strings"
	"testing"
)

func TestResolvedDSNPostgres(t *testing.T) {
	db := Database{
		Driver:   DriverPostgres,
		Host:     "127.0.0.1",
		Port:     5432,
		User:     "gowork",
		Password: "p a'ss",
		Name:     "gowork",
		Params:   map[string]string{"TimeZone": "Asia/Shanghai"},
	}

	got := db.ResolvedDSN()

	for _, want := range []string{
		"host=127.0.0.1",
		"port=5432",
		"user=gowork",
		"dbname=gowork",
		`password='p a\'ss'`, // 含空格与引号，必须被引起来并转义
		"sslmode=disable",    // 未显式配置时的兜底
		"TimeZone=Asia/Shanghai",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("postgres DSN 缺少 %q: %s", want, got)
		}
	}

	// 显式 dsn 优先，不做任何拼接。
	db.DSN = "postgres://custom"
	if got := db.ResolvedDSN(); got != "postgres://custom" {
		t.Errorf("显式 dsn 应直接返回，得到 %q", got)
	}
}

func TestResolvedDSNMySQL(t *testing.T) {
	db := Database{
		Driver:   DriverMySQL,
		Host:     "db.local",
		Port:     3307,
		User:     "root",
		Password: "secret",
		Name:     "erp",
	}

	got := db.ResolvedDSN()
	want := "root:secret@tcp(db.local:3307)/erp?"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("mysql DSN 前缀错误: %s", got)
	}
	for _, kv := range []string{"charset=utf8mb4", "parseTime=True", "loc=Local"} {
		if !strings.Contains(got, kv) {
			t.Errorf("mysql DSN 缺少默认参数 %q: %s", kv, got)
		}
	}
}

func TestResolvedDSNSQLite(t *testing.T) {
	if got := (Database{Driver: DriverSQLite, DSN: "file:./x.db"}).ResolvedDSN(); got != "file:./x.db" {
		t.Errorf("sqlite 显式 dsn 应原样返回，得到 %q", got)
	}

	got := (Database{Driver: DriverSQLite}).ResolvedDSN()
	if !strings.HasPrefix(got, "file:data/gowork.db?") {
		t.Errorf("sqlite 默认路径不对: %s", got)
	}
	for _, pragma := range []string{"busy_timeout", "journal_mode(WAL)"} {
		if !strings.Contains(got, pragma) {
			t.Errorf("sqlite 默认 DSN 缺少 pragma %q: %s", pragma, got)
		}
	}
}

func TestValidateDatabase(t *testing.T) {
	cases := []struct {
		name    string
		db      Database
		wantErr bool
	}{
		{"sqlite 允许只有文件", Database{Driver: DriverSQLite}, false},
		{"postgres 拆字段齐全", Database{Driver: DriverPostgres, Host: "h", User: "u", Name: "n"}, false},
		{"postgres 缺 user", Database{Driver: DriverPostgres, Host: "h", Name: "n"}, true},
		{"postgres 只有 dsn", Database{Driver: DriverPostgres, DSN: "x"}, false},
		{"未知驱动", Database{Driver: "oracle"}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.db.Validate()
			if c.wantErr != (err != nil) {
				t.Fatalf("wantErr=%v, got %v", c.wantErr, err)
			}
		})
	}
}

func TestValidateCache(t *testing.T) {
	on := true
	cases := []struct {
		name    string
		cache   Cache
		wantErr bool
	}{
		{"memory 默认落盘", Cache{Driver: CacheMemory, Memory: MemoryConfig{Persist: &on, Path: "data/x.snapshot"}}, false},
		{"memory 落盘但无路径", Cache{Driver: CacheMemory, Memory: MemoryConfig{Persist: &on}}, true},
		{"redis 缺 addr", Cache{Driver: CacheRedis}, true},
		{"redis 正常", Cache{Driver: CacheRedis, Redis: RedisConfig{Addr: "localhost:6379"}}, false},
		{"未知驱动", Cache{Driver: "memcached"}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cache.Validate()
			if c.wantErr != (err != nil) {
				t.Fatalf("wantErr=%v, got %v", c.wantErr, err)
			}
		})
	}
}
