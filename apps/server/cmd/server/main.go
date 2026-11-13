// GoWork 服务入口。
//
// 只负责：解析启动参数 -> 加载配置 -> 装配应用 -> 启停 HTTP 服务。
// 业务逻辑一律下沉到 internal 下的各层。
//
// 参数写法：全称用双横线，简称用单横线。三个参数都成对提供：
//
//	-c, --config       配置文件路径，缺省 configs/config.yaml
//	-e, --env-prefix   环境变量前缀，缺省 GOWORK_（见 internal/config）
//	-v, --version      打印版本信息然后退出
//
// 单横线的全称（-config）会被拒绝：标准库对单双横线一视同仁，不自己拦一道，
// 这条约定就只是注释里的一句话。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"gowork_server/internal/bootstrap"
	"gowork_server/internal/config"
)

// 版本号在构建期用 ldflags 注入，代码里不读文件：
//
//	go build -ldflags "-X main.version=0.1.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%FT%TZ)"
//
// 正本是仓库根目录的 VERSION.txt（`task release` 写它）。两条构建路径都把它的内容交给
// 上面的 -X：`task server:build`（缺省读 VERSION.txt，可用 APP_VERSION=1.2.3 覆盖）与
// 发布流水线（用 tag 名，那才是对外的权威）。
//
// 镜像那条路不读任何版本文件（后端镜像的上下文只有 apps/server）：版本号只经
// `--build-arg VERSION` 传，缺省落在 dev 上；本地 `task server:image` 传的是
// `git describe --always --long --dirty` 的结果。
//
// 为什么不在代码里读 VERSION：产物要能在任何目录、任何机器上自证，不该依赖旁边恰好
// 放着一份文本；而 -X 只对「字面量初始化的 string 变量」可靠，换成函数返回值后会在
// init 阶段把注进去的值静默覆盖掉，构建日志一个字都不会提。
var (
	// 裸 go build 与 air 热重载不带 ldflags，落在默认值上 —— 一眼看出不是发布产物。
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	opts, err := parseArgs(os.Args[1:])
	// --help 走的是 flag.ErrHelp：用法已经打印过，直接当成功退出。
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	// 放在加载配置之前：--version 不该依赖配置文件是否存在。
	if opts.version {
		fmt.Printf("gowork-server %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
		fmt.Printf("  go:     %s\n", runtime.Version())
		return nil
	}

	cfg, err := config.LoadWithEnvPrefix(opts.configPath, opts.envPrefix)
	if err != nil {
		return err
	}

	app, err := bootstrap.New(cfg)
	if err != nil {
		return err
	}
	defer app.Close()

	srv := &http.Server{
		Addr:    cfg.HTTP.Addr,
		Handler: app.Engine,
		// 读请求头的上限：挡住 slowloris 那类"慢慢发头、一直占着连接"的客户端。
		ReadHeaderTimeout: 10 * time.Second,
		// 读整个请求（含 body）的上限。比请求体上限（32MB）宽裕得多，
		// 它要挡的是"发得极慢"，不是"发得很大"——后者由 middleware.BodyLimit 管。
		ReadTimeout: 60 * time.Second,
		// 写完响应的时间上限：比单请求处理上限（30s）多留一段，
		// 免得 30 秒的活儿刚好在写响应时被掐断，客户端拿到半截 JSON。
		WriteTimeout: 35 * time.Second,
		// keep-alive 空闲连接的回收时间：浏览器复用连接，但不能永远留着。
		IdleTimeout: 120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		app.Log.Info("服务启动", "addr", cfg.HTTP.Addr, "env", cfg.App.Env, "cache", app.Cache.Driver())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case <-ctx.Done():
		app.Log.Info("收到退出信号，开始优雅关闭")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownDuration())
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("优雅关闭超时: %w", err)
	}
	app.Log.Info("服务已退出")
	return nil
}

// defaultConfigPath 是 --config 的缺省值，也是 --help 里那句话的来源。
const defaultConfigPath = "configs/config.yaml"

// optionSpec 描述一个启动参数：全称、简称、值占位符（空串表示布尔开关）与用法说明。
type optionSpec struct {
	short string
	long  string
	value string
	usage string
}

// optionSpecs 是参数表的唯一事实来源：注册、简称归一与 --help 的输出都从它推导，
// 免得「加了一个参数、忘了加简称」这类漂移要在三处比对才能发现。
var optionSpecs = []optionSpec{
	// 占位符用英文：--help 的对齐靠 tabwriter 按 rune 数算列宽，
	// 中文占两列却只算一个 rune，混进去这一列就歪了。
	{"c", "config", "<path>", "配置文件路径（缺省 " + defaultConfigPath + "）"},
	{"e", "env-prefix", "<prefix>", "环境变量前缀，缺省 GOWORK_（大小写不敏感，尾部下划线自动补）"},
	{"v", "version", "", "打印版本信息并退出（不读配置文件）"},
	{"h", "help", "", "打印本帮助并退出"},
}

// options 是解析后的启动参数。
type options struct {
	configPath string
	envPrefix  string
	version    bool
}

func parseArgs(args []string) (*options, error) {
	fs := flag.NewFlagSet("gowork-server", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { writeUsage(fs.Output(), fs.Name()) }

	// 只注册全称：简称在 normalizeArgs 里换成全称后再进来，
	// 一个值不挂两个 flag 名字，--help 也就不会把同一项印两遍。
	opts := &options{}
	fs.StringVar(&opts.configPath, "config", defaultConfigPath, "")
	fs.StringVar(&opts.envPrefix, "env-prefix", config.DefaultEnvPrefix, "")
	fs.BoolVar(&opts.version, "version", false, "")

	normalized, err := normalizeArgs(args)
	if err != nil {
		return nil, err
	}
	if err := fs.Parse(normalized); err != nil {
		return nil, err
	}
	return opts, nil
}

// normalizeArgs 落实「全称双横线、简称单横线」：把 -c 换成 --config，
// 单横线写全称（-config）直接拒绝。标准库本身两种写法都收，
// 不在这里拦一道，上面那条约定就只是注释里的一句话。
func normalizeArgs(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for i, arg := range args {
		// "--" 之后全是位置参数，原样透传；单独一个 "-" 也按位置参数处理。
		if arg == "--" {
			return append(out, args[i:]...), nil
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
			out = append(out, arg)
			continue
		}

		name, value, hasValue := strings.Cut(arg[1:], "=")
		spec := lookupShort(name)
		if spec == nil {
			if long := lookupLong(name); long != nil {
				return nil, fmt.Errorf("参数 -%s 是全称，要写成 --%s（简称 -%s）", name, long.long, long.short)
			}
			return nil, fmt.Errorf("未知参数 -%s：全称用 --，简称用 -，可用参数见 --help", name)
		}

		normalized := "--" + spec.long
		if hasValue {
			normalized += "=" + value
		}
		out = append(out, normalized)
	}
	return out, nil
}

func lookupShort(name string) *optionSpec {
	for i := range optionSpecs {
		if optionSpecs[i].short == name {
			return &optionSpecs[i]
		}
	}
	return nil
}

func lookupLong(name string) *optionSpec {
	for i := range optionSpecs {
		if optionSpecs[i].long == name {
			return &optionSpecs[i]
		}
	}
	return nil
}

func writeUsage(w io.Writer, name string) {
	fmt.Fprintf(w, "用法: %s [选项]\n\n选项:\n", name)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, s := range optionSpecs {
		head := "  -" + s.short + ", --" + s.long
		if s.value != "" {
			head += " " + s.value
		}
		fmt.Fprintf(tw, "%s\t%s\n", head, s.usage)
	}
	_ = tw.Flush()
}
