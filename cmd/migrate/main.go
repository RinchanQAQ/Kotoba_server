// Command migrate 是独立的数据库迁移命令，与 API 进程分开执行。
//
// 用法（常用命令已在 Makefile 中封装）：
//
//	go run ./cmd/migrate up            # 升级到最新版本
//	go run ./cmd/migrate down          # 回退一个版本（-steps 3 可回退多个）
//	go run ./cmd/migrate version       # 查看当前版本与 dirty 状态
//	go run ./cmd/migrate force 2       # 版本记录与库结构不一致时手工对齐
//	go run ./cmd/migrate drop          # 删除所有表（危险，仅限本地）
//
// 只依赖配置文件与 MySQL，不监听端口、不连 Redis，因此可以在容器编排里
// 作为一次性任务（init job）先于 API 启动。
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql" // 注册 database/sql 的 mysql 驱动
	"github.com/golang-migrate/migrate/v4"
	"go.uber.org/zap"

	"kotoba/internal/bootstrap"
	"kotoba/internal/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "迁移失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "配置文件路径，默认 configs/config.yaml（可用 KOTOBA_CONFIG 指定）")
	env := flag.String("env", "", "运行环境 development/test/production，默认取 KOTOBA_ENV / APP_ENV")
	steps := flag.Int("steps", 1, "down 时回退的版本数")
	flag.Parse()

	command := "up"
	args := flag.Args()
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}

	cfg, err := bootstrap.LoadConfig(bootstrap.LoadOptions{ConfigPath: *configPath, Env: *env})
	if err != nil {
		return err
	}

	logger, err := bootstrap.NewLogger(cfg.Log, cfg.App.Name)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()

	// 迁移脚本通常包含多条语句，必须开启 multiStatements；
	// 迁移使用独立连接池，避免与业务连接共享会话状态。
	sqlDB, err := sql.Open("mysql", cfg.MySQL.DSNWithParams(map[string]string{"multiStatements": "true"}))
	if err != nil {
		return fmt.Errorf("打开 MySQL 连接失败: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()
	sqlDB.SetMaxOpenConns(2)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("MySQL 连通性检查失败: %w", err)
	}

	m, err := migrations.New(sqlDB)
	if err != nil {
		return err
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			logger.Warn("关闭迁移器出错", zap.Error(srcErr), zap.Error(dbErr))
		}
	}()

	logger.Info("执行迁移命令",
		zap.String("command", command),
		zap.String("env", cfg.Env),
		zap.String("database", cfg.MySQL.Database),
	)

	switch command {
	case "up":
		err = m.Up()
	case "down":
		if *steps <= 0 {
			return errors.New("-steps 必须为正整数")
		}
		err = m.Steps(-*steps)
	case "version":
		return reportVersion(m, logger)
	case "force":
		return force(m, args, logger)
	case "drop":
		err = m.Drop()
	default:
		return fmt.Errorf("未知命令 %q（可选 up / down / version / force / drop）", command)
	}

	if err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("数据库已是最新版本，无需迁移")
			return reportVersion(m, logger)
		}
		return err
	}

	logger.Info("迁移执行成功")
	return reportVersion(m, logger)
}

// reportVersion 打印当前迁移版本，dirty 表示上一次迁移执行到一半失败。
func reportVersion(m *migrate.Migrate, logger *zap.Logger) error {
	version, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			logger.Info("当前数据库尚未应用任何迁移")
			return nil
		}
		return fmt.Errorf("读取迁移版本失败: %w", err)
	}

	logger.Info("当前迁移版本", zap.Uint("version", version), zap.Bool("dirty", dirty))
	if dirty {
		logger.Warn("数据库处于 dirty 状态：请人工确认库结构后执行 force <version> 对齐版本号")
	}
	return nil
}

// force 手工把版本号标记为指定值，用于修复 dirty 状态。
func force(m *migrate.Migrate, args []string, logger *zap.Logger) error {
	if len(args) == 0 {
		return errors.New("force 需要版本号参数，例如 go run ./cmd/migrate force 2")
	}

	version, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("版本号必须是整数: %q", args[0])
	}

	if err := m.Force(version); err != nil {
		return fmt.Errorf("强制设置迁移版本失败: %w", err)
	}

	logger.Info("已将迁移版本强制标记", zap.Int("version", version))
	return reportVersion(m, logger)
}
