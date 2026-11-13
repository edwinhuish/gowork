// Package config 负责加载与校验应用配置。
//
// 优先级：配置文件 < 环境变量（缺省前缀 GOWORK_，可用 LoadWithEnvPrefix 换掉）。
// 生产环境的密码、DSN 一律走环境变量，不进仓库。
//
// 存储相关的配置都遵循同一个思路：**用配置选项切换实现，而不是改代码**。
// 数据库靠 database.driver（postgres | mysql | sqlite），
// 缓存靠 cache.driver（redis | memory），两者互不牵连，可以任意组合。
package config

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"gowork_server/internal/pkg/dialect"
)

// 支持的数据库驱动。
//
// 取值定义在 pkg/dialect：配置校验、连接装配与方言分支共用同一组字面量，
// 这里只保留配置语义的名字。
const (
	DriverPostgres = dialect.Postgres
	DriverMySQL    = dialect.MySQL
	DriverSQLite   = dialect.SQLite
)

// 支持的缓存驱动。
const (
	CacheRedis  = "redis"
	CacheMemory = "memory"
)

// 各驱动的默认端口，未显式配置时使用。
var defaultPorts = map[string]int{
	DriverPostgres: 5432,
	DriverMySQL:    3306,
}

type Config struct {
	App      App      `yaml:"app"`
	HTTP     HTTP     `yaml:"http"`
	Database Database `yaml:"database"`
	Cache    Cache    `yaml:"cache"`
	JWT      JWT      `yaml:"jwt"`
	Log      Log      `yaml:"log"`
	PDF      PDF      `yaml:"pdf"`
}

// PDF 描述单据打印（报价单 / 形式发票）。
//
// 字体与模板都能指向外部文件：换版式、换字体不该要求重新编译。
// 两者留空时用内置模板与自动探测到的中文字体；探测不到也不阻止启动——
// 打印是附加能力，不该让整个服务起不来（PDF 接口会明确报缺什么）。
type PDF struct {
	// FontPath 是用于嵌入 PDF 的中文字体，必须是 TrueType（.ttf）。
	// .ttc 字体集合不被支持：那种文件里装了多套字，渲染时挑哪一套是猜的。
	FontPath string `yaml:"font_path"`
	// TemplateDir 非空时以目录里的 *.json 为准，改完下一次请求就生效。
	TemplateDir string `yaml:"template_dir"`
	// 抬头信息，模板里用 {company_name} / {company_address} / {company_contact} 引用。
	CompanyName    string `yaml:"company_name"`
	CompanyAddress string `yaml:"company_address"`
	CompanyContact string `yaml:"company_contact"`
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

// Database 描述数据库连接。
//
// 两种写法二选一：
//  1. 直接给 dsn —— 完全掌控连接串，适合云厂商托管的特殊参数；
//  2. 拆成 host/port/user/password/name + params —— 换驱动时只改 driver 一个字段，
//     剩下的字段名不变（postgres 与 mysql 共用同一组键），避免换库就得重写连接串。
type Database struct {
	Driver string `yaml:"driver"` // postgres | mysql | sqlite
	DSN    string `yaml:"dsn"`    // 显式连接串，非空时忽略下面的拆分字段
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
	// Name 是要连的库名；登录凭据是紧跟其后的 username + password。
	// 三者按"库名 → 账号 → 密码"排列，样例配置里也是这个顺序：
	// 把 name 与 username 摆在一起时，最容易被读成"这个库的账号叫什么"。
	Name     string `yaml:"name"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// Params 是附加连接参数，语义按驱动解释：
	// postgres 拼成 key=value（sslmode、search_path…），mysql 拼成 query（charset、loc…）。
	Params map[string]string `yaml:"params"`

	MaxOpen     int  `yaml:"max_open"`
	MaxIdle     int  `yaml:"max_idle"`
	MaxLifetime int  `yaml:"max_lifetime"` // 秒
	AutoMigrate bool `yaml:"auto_migrate"`
	// DemoSeed 决定是否灌演示数据：示例主数据（往来单位 / 产品 / 货源 / 条款模板）
	// 与整条业务演示链（报价 / 合同 / 订单 / 出货 / 应收 / 收付款 / 库存）。
	//
	// 基础数据不在这里面——默认管理员与币种汇率**总是**在空表时写入，没有开关。
	// 曾经有个 seed 开关连基础数据一起管，结果关掉它之后空库连登录都进不去。
	//
	// 演示数据默认关闭、只有本地开发配置打开：测试断言的是"自己造的数据"的合计，
	// 库里预置几十张单会让它们全部对不上。
	DemoSeed bool `yaml:"demo_seed"`
	// LogLevel: silent | error | warn | info，默认 warn（只打慢查询与错误）。
	LogLevel string `yaml:"log_level"`
}

// Cache 描述缓存实现。
type Cache struct {
	Driver string `yaml:"driver"` // redis | memory
	// Prefix 统一加在业务 key 前，两套实现都生效，切换驱动后 key 语义不变。
	Prefix string `yaml:"prefix"`
	// FallbackToMemory 仅在 driver=redis 时有意义：连接失败是否退回内存实现。
	// 生产应设为 false —— 带病降级会让人误以为多实例共享还在生效。
	FallbackToMemory bool         `yaml:"fallback_to_memory"`
	Redis            RedisConfig  `yaml:"redis"`
	Memory           MemoryConfig `yaml:"memory"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	// DialTimeout 连接与读写超时（秒），默认 2。
	DialTimeout int `yaml:"dial_timeout"`
}

// MemoryConfig 控制进程内缓存的行为。
//
// 内存实现支持落盘：退出时把非过期数据写进快照，下次启动加载回来。
// 这样开发环境不用起 Redis，重启后登录态、限流计数也不会突然清零。
type MemoryConfig struct {
	// Persist 是否落盘，默认 true（仅 driver=memory 生效）。
	Persist *bool `yaml:"persist"`
	// Path 快照文件路径，默认 ./data/cache.snapshot；目录不存在会自动创建。
	Path string `yaml:"path"`
	// SaveInterval 定时落盘间隔（秒），默认 30。
	// 每次写操作只置脏标记，真正的磁盘 IO 由这个间隔合并，避免 Set 变成同步写盘。
	SaveInterval int `yaml:"save_interval"`
}

type JWT struct {
	Secret string `yaml:"secret"`
	Expire int    `yaml:"expire"` // 小时
}

type Log struct {
	Level  string `yaml:"level"`  // debug | info | warn | error
	Format string `yaml:"format"` // text | json
}

// DefaultEnvPrefix 是环境变量前缀的缺省值：GOWORK_APP_ENV 即前缀 + APP_ENV。
const DefaultEnvPrefix = "GOWORK_"

// Load 读取配置文件并应用默认值与环境变量覆盖（环境变量用缺省前缀）。
func Load(path string) (*Config, error) {
	return LoadWithEnvPrefix(path, DefaultEnvPrefix)
}

// LoadWithEnvPrefix 同 Load，但允许指定环境变量前缀。
//
// 前缀为空等价于缺省值；不以 "_" 结尾时自动补上，大小写不敏感
// （"--env-prefix acme" 与 "--env-prefix ACME_" 等价，都读 ACME_APP_ENV）。
//
// 需要它的场景是同一个制品部署多套实例：同一个二进制，靠前缀区分
// 各套的环境变量，不必为了换一套变量名再编译一次。
func LoadWithEnvPrefix(path, prefix string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s: %w", path, err)
	}

	cfg.applyDefaults()
	cfg.applyEnv(NormalizeEnvPrefix(prefix))

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// NormalizeEnvPrefix 把用户给的前缀规整成 "大写 + 尾部下划线" 的唯一形式，
// 空串返回 DefaultEnvPrefix。
func NormalizeEnvPrefix(prefix string) string {
	p := strings.ToUpper(strings.TrimSpace(prefix))
	if p == "" {
		return DefaultEnvPrefix
	}
	if !strings.HasSuffix(p, "_") {
		p += "_"
	}
	return p
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
		c.Database.Driver = DriverSQLite
	}
	if c.Database.Port == 0 {
		c.Database.Port = defaultPorts[c.Database.Driver]
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
	if c.Database.LogLevel == "" {
		c.Database.LogLevel = "warn"
	}

	if c.Cache.Driver == "" {
		c.Cache.Driver = CacheMemory
	}
	if c.Cache.Prefix == "" {
		c.Cache.Prefix = "gowork:"
	}
	if c.Cache.Redis.DialTimeout == 0 {
		c.Cache.Redis.DialTimeout = 2
	}
	if c.Cache.Memory.Persist == nil {
		c.Cache.Memory.Persist = boolPtr(true)
	}
	if c.Cache.Memory.Path == "" {
		c.Cache.Memory.Path = "data/cache.snapshot"
	}
	if c.Cache.Memory.SaveInterval == 0 {
		c.Cache.Memory.SaveInterval = 30
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

	// 抬头给一套占位值：不填也能打印，拿到 PDF 的人一眼能看出这是样例抬头，
	// 比留空印出一张没有主体的单据更容易发现该改配置。
	if c.PDF.CompanyName == "" {
		c.PDF.CompanyName = "GoWork Trading Co., Ltd."
	}
	if c.PDF.CompanyAddress == "" {
		c.PDF.CompanyAddress = "Ningbo, Zhejiang, China"
	}
	if c.PDF.CompanyContact == "" {
		c.PDF.CompanyContact = "sales@gowork.example"
	}
}

// applyEnv 只覆盖最常因环境而变的几项，其余仍以配置文件为准。
// prefix 已由 NormalizeEnvPrefix 规整（大写且以 "_" 结尾）。
func (c *Config) applyEnv(prefix string) {
	// 所有变量名都经 key 拼出来：换前缀时只改这一处，漏改会立刻被测试发现。
	key := func(name string) string { return prefix + name }

	setStr(&c.App.Env, key("APP_ENV"))
	setStr(&c.HTTP.Addr, key("HTTP_ADDR"))
	setStr(&c.HTTP.Mode, key("HTTP_MODE"))
	setStr(&c.Database.Driver, key("DB_DRIVER"))
	setStr(&c.Database.DSN, key("DB_DSN"))
	setStr(&c.Database.Host, key("DB_HOST"))
	setStr(&c.Database.Name, key("DB_NAME"))
	setStr(&c.Database.Username, key("DB_USERNAME"))
	setStr(&c.Database.Password, key("DB_PASSWORD"))
	setStr(&c.Database.LogLevel, key("DB_LOG_LEVEL"))
	setStr(&c.Cache.Driver, key("CACHE_DRIVER"))
	setStr(&c.Cache.Memory.Path, key("CACHE_MEMORY_PATH"))
	setStr(&c.Cache.Redis.Addr, key("REDIS_ADDR"))
	setStr(&c.Cache.Redis.Password, key("REDIS_PASSWORD"))
	setStr(&c.JWT.Secret, key("JWT_SECRET"))
	setStr(&c.Log.Level, key("LOG_LEVEL"))
	setStr(&c.Log.Format, key("LOG_FORMAT"))
	setStr(&c.PDF.FontPath, key("PDF_FONT_PATH"))
	setStr(&c.PDF.TemplateDir, key("PDF_TEMPLATE_DIR"))
	setStr(&c.PDF.CompanyName, key("PDF_COMPANY_NAME"))

	setInt(&c.Database.Port, key("DB_PORT"))
	setInt(&c.Cache.Redis.DB, key("REDIS_DB"))
	setInt(&c.Cache.Memory.SaveInterval, key("CACHE_SAVE_INTERVAL"))
	setBool(&c.Cache.FallbackToMemory, key("CACHE_FALLBACK_TO_MEMORY"))
	setBoolPtr(&c.Cache.Memory.Persist, key("CACHE_MEMORY_PERSIST"))

	if v := os.Getenv(key("HTTP_CORS_ORIGINS")); v != "" {
		c.HTTP.CORSOrigins = strings.Split(v, ",")
	}
}

// Validate 校验启动必需项，把配置问题暴露在启动阶段而不是首次请求。
func (c *Config) Validate() error {
	if err := c.Database.Validate(); err != nil {
		return err
	}
	if err := c.Cache.Validate(); err != nil {
		return err
	}

	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret 不能为空")
	}
	if c.App.Env == "prod" && len(c.JWT.Secret) < 32 {
		return fmt.Errorf("生产环境 jwt.secret 至少 32 位")
	}
	return nil
}

func (d Database) Validate() error {
	switch d.Driver {
	case DriverSQLite:
		// sqlite 允许 dsn 与 name 都为空，此时走默认文件路径。
	case DriverPostgres, DriverMySQL:
		if d.DSN != "" {
			return nil
		}
		if d.Host == "" || d.Name == "" || d.Username == "" {
			return fmt.Errorf("database.driver=%s 时需要 dsn，或同时配置 host / name / username", d.Driver)
		}
	default:
		return fmt.Errorf("不支持的 database.driver: %s（可选 %s / %s / %s）",
			d.Driver, DriverPostgres, DriverMySQL, DriverSQLite)
	}
	return nil
}

func (c Cache) Validate() error {
	switch c.Driver {
	case CacheMemory:
		if c.Memory.Persist != nil && *c.Memory.Persist && c.Memory.Path == "" {
			return fmt.Errorf("cache.driver=memory 且开启持久化时必须配置 cache.memory.path")
		}
	case CacheRedis:
		if c.Redis.Addr == "" {
			return fmt.Errorf("cache.driver=redis 时必须配置 cache.redis.addr")
		}
	default:
		return fmt.Errorf("不支持的 cache.driver: %s（可选 %s / %s）", c.Driver, CacheRedis, CacheMemory)
	}
	return nil
}

// ResolvedDSN 返回实际使用的连接串。
// 显式配置了 dsn 就用它；否则按驱动把拆分字段拼成该驱动认可的格式。
func (d Database) ResolvedDSN() string {
	if d.DSN != "" {
		return d.DSN
	}

	switch d.Driver {
	case DriverPostgres:
		kv := map[string]string{
			"host":   d.Host,
			"port":   strconv.Itoa(d.Port),
			"user":   d.Username,
			"dbname": d.Name,
		}
		if d.Password != "" {
			kv["password"] = d.Password
		}
		// 生产默认要求 TLS，本地容器没证书，这里只给"没配就用 disable"的兜底。
		if _, ok := d.Params["sslmode"]; !ok {
			kv["sslmode"] = "disable"
		}
		for k, v := range d.Params {
			kv[k] = v
		}
		return postgresDSN(kv)

	case DriverMySQL:
		query := url.Values{}
		query.Set("charset", "utf8mb4")
		query.Set("parseTime", "True")
		query.Set("loc", "Local")
		for k, v := range d.Params {
			query.Set(k, v)
		}
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
			d.Username, d.Password, d.Host, d.Port, d.Name, query.Encode())

	default:
		// sqlite：dsn 已在上方返回；走到这里说明只想给个文件名。
		file := d.Name
		if file == "" {
			file = "data/gowork.db"
		}
		// WAL + busy_timeout 是并发读写下最实用的两个 pragma，三库行为对齐时也要靠它。
		return fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", file)
	}
}

// postgresDSN 拼 key=value 形式的连接串，含空格或引号的值需要加单引号。
func postgresDSN(kv map[string]string) string {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := kv[k]
		if v == "" || strings.ContainsAny(v, " '\\") {
			v = "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

// IsProd 判断是否生产环境。
func (c *Config) IsProd() bool { return c.App.Env == "prod" }

// ShutdownDuration 返回优雅退出的等待时长。
func (c *Config) ShutdownDuration() time.Duration {
	return time.Duration(c.HTTP.ShutdownTimeout) * time.Second
}

// MemoryPersistEnabled 判断内存缓存是否落盘。
func (m MemoryConfig) PersistEnabled() bool {
	return m.Persist != nil && *m.Persist && m.Path != ""
}

func boolPtr(b bool) *bool { return &b }

func setStr(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func setInt(dst *int, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

func setBool(dst *bool, key string) {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*dst = b
		}
	}
}

// setBoolPtr 处理三态开关（未配置 / true / false），用于 persist 这类默认开启的选项。
func setBoolPtr(dst **bool, key string) {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*dst = &b
		}
	}
}
