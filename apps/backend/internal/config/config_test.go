package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeEnvPrefix(t *testing.T) {
	cases := map[string]string{
		"":        DefaultEnvPrefix,
		"   ":     DefaultEnvPrefix,
		"GOWORK_": DefaultEnvPrefix,
		"acme":    "ACME_",
		"acme_":   "ACME_",
		" Acme_ ": "ACME_",
	}
	for in, want := range cases {
		if got := NormalizeEnvPrefix(in); got != want {
			t.Errorf("NormalizeEnvPrefix(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestLoadWithEnvPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("jwt:\n  secret: from-file\n"), 0o600); err != nil {
		t.Fatalf("写临时配置: %v", err)
	}

	t.Setenv("GOWORK_JWT_SECRET", "from-gowork")
	t.Setenv("ACME_JWT_SECRET", "from-acme")

	// 自定义前缀只读自己那套变量。
	cfg, err := LoadWithEnvPrefix(path, "acme")
	if err != nil {
		t.Fatalf("LoadWithEnvPrefix: %v", err)
	}
	if cfg.JWT.Secret != "from-acme" {
		t.Errorf("自定义前缀应读 ACME_JWT_SECRET，得到 %q", cfg.JWT.Secret)
	}

	// 不指定前缀（含显式空串）时行为与原来一致。
	for _, prefix := range []string{DefaultEnvPrefix, ""} {
		cfg, err := LoadWithEnvPrefix(path, prefix)
		if err != nil {
			t.Fatalf("LoadWithEnvPrefix(%q): %v", prefix, err)
		}
		if cfg.JWT.Secret != "from-gowork" {
			t.Errorf("prefix=%q 应读 GOWORK_JWT_SECRET，得到 %q", prefix, cfg.JWT.Secret)
		}
	}

	// 环境变量都没有时回落到配置文件。
	os.Unsetenv("GOWORK_JWT_SECRET")
	cfg, err = Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.JWT.Secret != "from-file" {
		t.Errorf("无环境变量时应读配置文件，得到 %q", cfg.JWT.Secret)
	}
}

func TestLoadDatabaseNameAndUsername(t *testing.T) {
	// 库里配了这个变量时会整段覆盖 dsn，把本用例的断言搅乱。
	t.Setenv("GOWORK_DB_DSN", "")

	path := filepath.Join(t.TempDir(), "config.yaml")
	body := `
jwt:
  secret: test-secret
database:
  driver: postgres
  host: db.local
  name: erp
  username: gowork
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写临时配置: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// name 是库名、username 是账号：各自落位，不会互相串。
	if cfg.Database.Name != "erp" {
		t.Errorf("name 未读到，得到 %q", cfg.Database.Name)
	}
	if cfg.Database.Username != "gowork" {
		t.Errorf("username 未读到，得到 %q", cfg.Database.Username)
	}
	dsn := cfg.Database.ResolvedDSN()
	for _, want := range []string{"dbname=erp", "user=gowork"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("连接串缺少 %q: %s", want, dsn)
		}
	}
}

func TestResolvedDSNPostgres(t *testing.T) {
	db := Database{
		Driver:   DriverPostgres,
		Host:     "127.0.0.1",
		Port:     5432,
		Name:     "gowork",
		Username: "gowork",
		Password: "p a'ss",
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
		Name:     "erp",
		Username: "root",
		Password: "secret",
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
		{"postgres 拆字段齐全", Database{Driver: DriverPostgres, Host: "h", Name: "n", Username: "u"}, false},
		{"postgres 缺 username", Database{Driver: DriverPostgres, Host: "h", Name: "n"}, true},
		{"postgres 缺 name", Database{Driver: DriverPostgres, Host: "h", Username: "u"}, true},
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
