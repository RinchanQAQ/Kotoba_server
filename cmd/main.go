package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"kotoba/internal/app"
	"kotoba/internal/bootstrap"
)

// 构建信息，可通过 -ldflags "-X main.version=... -X main.buildTime=..." 注入。
var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "服务退出: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "配置文件路径，默认 configs/config.yaml（可用 KOTOBA_CONFIG 指定）")
	env := flag.String("env", "", "运行环境 development/test/production，默认取 KOTOBA_ENV / APP_ENV")
	showVersion := flag.Bool("version", false, "打印版本信息后退出")
	healthCheck := flag.Bool("health-check", false, "探测本机 /healthz 并以探测结果作为退出码（供容器 HEALTHCHECK 使用）")
	flag.Parse()

	if *showVersion {
		fmt.Printf("kotoba %s (built %s)\n", version, buildTime)
		return nil
	}

	opts := bootstrap.LoadOptions{ConfigPath: *configPath, Env: *env}

	if *healthCheck {
		return runHealthCheck(opts)
	}

	application, err := bootstrap.New(opts)
	if err != nil {
		return err
	}
	defer func() {
		if err := application.Close(); err != nil {
			application.Logger.Error("释放资源失败", zap.Error(err))
		}
	}()

	log := application.Logger
	log.Info("服务启动中", zap.String("version", version), zap.String("build_time", buildTime))

	srv := &http.Server{
		Addr:         application.Config.App.Addr,
		Handler:      app.NewRouter(application),
		ReadTimeout:  application.Config.App.ReadTimeout,
		WriteTimeout: application.Config.App.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case sig := <-quit:
		log.Info("收到退出信号，开始优雅关闭", zap.String("signal", sig.String()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), application.Config.App.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("HTTP 服务优雅关闭失败: %w", err)
	}

	log.Info("服务已退出")
	return nil
}

// runHealthCheck 请求本机 /healthz，成功返回 nil。
//
// 有了这个自检入口，镜像可以基于 distroless（无 shell、无 curl）构建，
// 容器 HEALTHCHECK 直接执行 api -health-check 即可。
func runHealthCheck(opts bootstrap.LoadOptions) error {
	cfg, err := bootstrap.LoadConfig(opts)
	if err != nil {
		return err
	}

	_, port, err := net.SplitHostPort(cfg.App.Addr)
	if err != nil {
		return fmt.Errorf("解析监听地址 %q 失败: %w", cfg.App.Addr, err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%s/healthz", port)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("健康检查请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("健康检查返回状态码 %d", resp.StatusCode)
	}

	return nil
}
