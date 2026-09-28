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
func New(configPath string) (*App, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	logger, err := NewLogger(cfg.Log, cfg.App.Name)
	if err != nil {
		return nil, err
	}

	db, err := NewMySQL(cfg.MySQL, logger)
	if err != nil {
		return nil, err
	}

	rdb, err := NewRedis(cfg.Redis, logger)
	if err != nil {
		_ = closeMySQL(db)
		return nil, err
	}

	logger.Info("应用依赖初始化完成",
		zap.String("env", cfg.App.Env),
		zap.String("addr", cfg.App.Addr),
	)

	return &App{Config: cfg, Logger: logger, MySQL: db, Redis: rdb}, nil
}

// Close 逆序释放已建立的连接。
func (a *App) Close() error {
	var firstErr error

	if err := closeMySQL(a.MySQL); err != nil {
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

func closeMySQL(db *gorm.DB) error {
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