package bootstrap

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// NewMySQL 建立数据库连接、配置连接池并做一次连通性检查。
func NewMySQL(cfg MySQLConfig, log *zap.Logger) (*gorm.DB, error) {
	db, err := gorm.Open(gormmysql.Open(cfg.DSN()), &gorm.Config{
		// SQL 日志经 zap 输出（与业务日志同格式、同 request_id），级别由 mysql.log_level 控制。
		Logger:                 newGormLogger(log, cfg.LogLevel, defaultSlowQueryThreshold),
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 SQL 连接池失败: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("MySQL 连通性检查失败: %w", err)
	}

	log.Info("MySQL 连接就绪",
		zap.String("addr", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))),
		zap.String("database", cfg.Database),
	)

	return db, nil
}

func gormLogLevel(s string) gormlogger.LogLevel {
	switch strings.ToLower(s) {
	case "silent":
		return gormlogger.Silent
	case "error":
		return gormlogger.Error
	case "warn":
		return gormlogger.Warn
	default:
		return gormlogger.Info
	}
}
