package bootstrap

import (
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// App 持有应用启动期构建的全部共享依赖。
type App struct {
	Config *Config
	Logger *zap.Logger
	MySQL  *gorm.DB
	Redis  *redis.Client
}

// New 加载配置并依次初始化日志、数据库、缓存。
//
// 依赖按「配置 → 日志 → 数据库 → 缓存」的顺序构建：
// 日志最先可用，后续每一步的失败原因都能被结构化记录下来。
func New(opts LoadOptions) (*App, error) {
	cfg, err := LoadConfig(opts)
	if err != nil {
		return nil, err
	}

	logger, err := NewLogger(cfg.Log, cfg.App.Name)
	if err != nil {
		return nil, err
	}

	// 记录配置来源，排查「到底加载了哪份配置」时非常有用。
	logger.Info("配置加载完成",
		zap.String("env", cfg.Env),
		zap.String("addr", cfg.App.Addr),
		zap.Strings("config_sources", cfg.Sources),
	)

	db, err := NewMySQL(cfg.MySQL, logger)
	if err != nil {
		return nil, err
	}

	rdb, err := NewRedis(cfg.Redis, logger)
	if err != nil {
		_ = CloseMySQL(db)
		return nil, err
	}

	logger.Info("应用依赖初始化完成", zap.String("env", cfg.App.Env))

	return &App{Config: cfg, Logger: logger, MySQL: db, Redis: rdb}, nil
}

// Close 逆序释放已建立的连接。
func (a *App) Close() error {
	var firstErr error

	if err := CloseMySQL(a.MySQL); err != nil {
		firstErr = err
	}

	if a.Redis != nil {
		if err := a.Redis.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("关闭 Redis 失败: %w", err)
		}
	}

	if a.Logger != nil {
		_ = a.Logger.Sync()
	}

	return firstErr
}

// CloseMySQL 关闭 gorm 底层的连接池。
// 同时被 App.Close 与 cmd/migrate 复用（迁移进程不需要 Redis）。
func CloseMySQL(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取 SQL 连接池失败: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("关闭 MySQL 失败: %w", err)
	}
	return nil
}
