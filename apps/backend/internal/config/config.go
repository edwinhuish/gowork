// Package config 负责加载与校验应用配置。
//
// 优先级：配置文件 < 环境变量（GOWORK_ 前缀）。
// 生产环境的密码、DSN 一律走环境变量，不进仓库。
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App      App      `yaml:"app"`
	HTTP     HTTP     `yaml:"http"`
	Database Database `yaml:"database"`
	Redis    Redis    `yaml:"redis"`
	JWT      JWT      `yaml:"jwt"`
	Log      Log      `yaml:"log"`
}

type App struct {
	Name string `yaml:"name"`
	Env  string `yaml:"env"` // dev | prod
}

type HTTP struct {
	Addr            string   `yaml:"addr"`
	Mode            string   `yaml:"mode"` // debug | release
	CORSOrigins     []string `yaml:"cors_origins"`
	ShutdownTimeout int      `yaml:"shutdown_timeout"` // 秒
}

type Database struct {
	Driver      string `yaml:"driver"` // mysql | sqlite
	DSN         string `yaml:"dsn"`
	MaxOpen     int    `yaml:"max_open"`
	MaxIdle     int    `yaml:"max_idle"`
	MaxLifetime int    `yaml:"max_lifetime"` // 秒
	AutoMigrate bool   `yaml:"auto_migrate"`
	Seed        bool   `yaml:"seed"`
}

type Redis struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	Prefix   string `yaml:"prefix"`
	// FallbackToMemory 在 Redis 不可用时退回进程内缓存，保证本地开发零依赖可跑。
	FallbackToMemory bool `yaml:"fallback_to_memory"`
}

type JWT struct {
	Secret string `yaml:"secret"`
	Expire int    `yaml:"expire"` // 小时
}

type Log struct {
	Level  string `yaml:"level"`  // debug | info | warn | error
	Format string `yaml:"format"` // text | json
}

// Load 读取配置文件并应用默认值与环境变量覆盖。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s: %w", path, err)
	}

	cfg.applyDefaults()
	cfg.applyEnv()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.App.Name == "" {
		c.App.Name = "gowork"
	}
	if c.App.Env == "" {
		c.App.Env = "dev"
	}
	if c.HTTP.Addr == "" {
		c.HTTP.Addr = ":8080"
	}
	if c.HTTP.Mode == "" {
		c.HTTP.Mode = "debug"
	}
	if c.HTTP.ShutdownTimeout == 0 {
		c.HTTP.ShutdownTimeout = 10
	}
	if c.Database.Driver == "" {
		c.Database.Driver = "sqlite"
	}
	if c.Database.MaxOpen == 0 {
		c.Database.MaxOpen = 50
	}
	if c.Database.MaxIdle == 0 {
		c.Database.MaxIdle = 10
	}
	if c.Database.MaxLifetime == 0 {
		c.Database.MaxLifetime = 3600
	}
	if c.JWT.Expire == 0 {
		c.JWT.Expire = 24
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "text"
	}
}

// applyEnv 只覆盖最常因环境而变的几项，其余仍以配置文件为准。
func (c *Config) applyEnv() {
	setStr(&c.App.Env, "GOWORK_APP_ENV")
	setStr(&c.HTTP.Addr, "GOWORK_HTTP_ADDR")
	setStr(&c.HTTP.Mode, "GOWORK_HTTP_MODE")
	setStr(&c.Database.Driver, "GOWORK_DB_DRIVER")
	setStr(&c.Database.DSN, "GOWORK_DB_DSN")
	setStr(&c.Redis.Addr, "GOWORK_REDIS_ADDR")
	setStr(&c.Redis.Password, "GOWORK_REDIS_PASSWORD")
	setStr(&c.JWT.Secret, "GOWORK_JWT_SECRET")
	setStr(&c.Log.Level, "GOWORK_LOG_LEVEL")
	setStr(&c.Log.Format, "GOWORK_LOG_FORMAT")

	if v := os.Getenv("GOWORK_REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Redis.DB = n
		}
	}
	if v := os.Getenv("GOWORK_HTTP_CORS_ORIGINS"); v != "" {
		c.HTTP.CORSOrigins = strings.Split(v, ",")
	}
}

// Validate 校验启动必需项，把配置问题暴露在启动阶段而不是首次请求。
func (c *Config) Validate() error {
	switch c.Database.Driver {
	case "mysql":
		if c.Database.DSN == "" {
			return fmt.Errorf("database.driver=mysql 时必须配置 database.dsn")
		}
	case "sqlite":
		if c.Database.DSN == "" {
			return fmt.Errorf("database.driver=sqlite 时必须配置 database.dsn")
		}
	default:
		return fmt.Errorf("不支持的 database.driver: %s", c.Database.Driver)
	}

	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret 不能为空")
	}
	if c.App.Env == "prod" && len(c.JWT.Secret) < 32 {
		return fmt.Errorf("生产环境 jwt.secret 至少 32 位")
	}
	return nil
}

// IsProd 判断是否生产环境。
func (c *Config) IsProd() bool { return c.App.Env == "prod" }

// ShutdownDuration 返回优雅退出的等待时长。
func (c *Config) ShutdownDuration() time.Duration {
	return time.Duration(c.HTTP.ShutdownTimeout) * time.Second
}

func setStr(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}
