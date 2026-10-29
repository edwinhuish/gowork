// GoWork 服务入口。
//
// 只负责：解析启动参数 -> 加载配置 -> 装配应用 -> 启停 HTTP 服务。
// 业务逻辑一律下沉到 internal 下的各层。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edwinhuish/gowork/internal/bootstrap"
	"github.com/edwinhuish/gowork/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
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
