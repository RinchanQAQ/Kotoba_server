package bootstrap

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"kotoba/internal/shared/logging"
)

// defaultSlowQueryThreshold 超过该耗时的 SQL 会以 warn 级别记录。
const defaultSlowQueryThreshold = 200 * time.Millisecond

// gormZapLogger 把 gorm 的 SQL 日志接入统一的 zap logger。
//
// 关键点：gorm 会把 db.WithContext(ctx) 传入的 context 一并交给 Trace，
// 因此只要 handler 使用 db.WithContext(c.Request.Context())，
// SQL 日志就会自动带上同一个 request_id，与 HTTP 访问日志串联。
type gormZapLogger struct {
	base  *zap.Logger
	level gormlogger.LogLevel
	slow  time.Duration
}

// newGormLogger 依据配置创建接入 zap 的 gorm logger。
func newGormLogger(base *zap.Logger, level string, slow time.Duration) gormlogger.Interface {
	if base == nil {
		base = zap.NewNop()
	}
	if slow <= 0 {
		slow = defaultSlowQueryThreshold
	}
	return &gormZapLogger{base: base, level: gormLogLevel(level), slow: slow}
}

// LogMode 返回调整了日志级别的新实例，gorm 内部会用它切换级别。
func (l *gormZapLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	clone := *l
	clone.level = level
	return &clone
}

func (l *gormZapLogger) Info(ctx context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Info {
		l.logger(ctx).Sugar().Debugf(msg, data...)
	}
}

func (l *gormZapLogger) Warn(ctx context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Warn {
		l.logger(ctx).Sugar().Warnf(msg, data...)
	}
}

func (l *gormZapLogger) Error(ctx context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Error {
		l.logger(ctx).Sugar().Errorf(msg, data...)
	}
}

// Trace 记录单条 SQL 的执行情况：错误、慢查询、普通调试信息。
func (l *gormZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}

	reqLog := l.logger(ctx)
	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := []zap.Field{
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
		zap.String("sql", sql),
	}

	switch {
	// 记录不存在属于正常的业务分支，不作为错误上报。
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		reqLog.Error("SQL 执行失败", append(fields, zap.Error(err))...)
	case elapsed > l.slow:
		reqLog.Warn("SQL 慢查询", fields...)
	case l.level >= gormlogger.Info:
		reqLog.Debug("SQL 执行", fields...)
	}
}

func (l *gormZapLogger) logger(ctx context.Context) *zap.Logger {
	return logging.FromOr(ctx, l.base)
}
