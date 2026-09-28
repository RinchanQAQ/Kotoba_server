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
	configPath := flag.String("config", "", "配置文件路径，默认 configs/config.yaml")
	showVersion := flag.Bool("version", false, "打印版本信息后退出")
	flag.Parse()

	if *showVersion {
		fmt.Printf("kotoba %s (built %s)\n", version, buildTime)
		return nil
	}

	path := *configPath
	if path == "" {
		path = os.Getenv(bootstrap.EnvConfigPath)
	}

	application, err := bootstrap.New(path)
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
