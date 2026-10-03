package bootstrap

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"kotoba/internal/shared/constant"
	"kotoba/internal/shared/logging"
)

func TestGormLoggerUsesRequestScopedLogger(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	base := zap.New(core)

	// 模拟 handler 中 db.WithContext(c.Request.Context()) 的行为。
	ctx := logging.WithLogger(context.Background(), base.With(zap.String(constant.FieldRequestID, "req-1")))

	newGormLogger(base, "info", time.Second).Trace(ctx,
		time.Now().Add(-5*time.Millisecond),
		func() (string, int64) { return "SELECT * FROM users", 3 },
		nil,
	)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("应记录 1 条 SQL 日志，实际 %d", len(entries))
	}

	fields := entries[0].ContextMap()
	if fields[constant.FieldRequestID] != "req-1" {
		t.Fatalf("SQL 日志应带 request_id，实际 %v", fields[constant.FieldRequestID])
	}
	if fields["sql"] != "SELECT * FROM users" {
		t.Fatalf("SQL 日志应包含语句，实际 %v", fields["sql"])
	}
	if fields["rows"] != int64(3) {
		t.Fatalf("SQL 日志应包含影响行数，实际 %v", fields["rows"])
	}
	if entries[0].Level != zapcore.DebugLevel {
		t.Fatalf("普通 SQL 应以 debug 级别记录，实际 %v", entries[0].Level)
	}
}

func TestGormLoggerFallsBackWithoutRequestContext(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)

	// 后台任务没有请求上下文，日志应回落到基础 logger 而不是被丢弃。
	newGormLogger(zap.New(core), "info", time.Second).Trace(context.Background(),
		time.Now(),
		func() (string, int64) { return "SELECT 1", 1 },
		nil,
	)

	if len(logs.All()) != 1 {
		t.Fatalf("缺少请求上下文时应回落到基础 logger，实际 %d 条", len(logs.All()))
	}
}

func TestGormLoggerSilentLevel(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)

	newGormLogger(zap.New(core), "silent", time.Second).Trace(context.Background(),
		time.Now(),
		func() (string, int64) { return "SELECT 1", 1 },
		nil,
	)

	if len(logs.All()) != 0 {
		t.Fatalf("silent 级别不应输出 SQL 日志，实际 %d 条", len(logs.All()))
	}
}

func TestGormLoggerWarnsSlowQuery(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)

	newGormLogger(zap.New(core), "warn", 10*time.Millisecond).Trace(context.Background(),
		time.Now().Add(-time.Second),
		func() (string, int64) { return "SELECT SLEEP(1)", 1 },
		nil,
	)

	entries := logs.All()
	if len(entries) != 1 || entries[0].Level != zapcore.WarnLevel {
		t.Fatalf("慢查询应以 warn 级别记录，实际 %+v", entries)
	}
}

func TestGormLoggerIgnoresRecordNotFound(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)

	// 记录不存在是正常业务分支，不应上报为错误。
	newGormLogger(zap.New(core), "info", time.Second).Trace(context.Background(),
		time.Now(),
		func() (string, int64) { return "SELECT * FROM users WHERE id = 1", 0 },
		gorm.ErrRecordNotFound,
	)

	for _, entry := range logs.All() {
		if entry.Level >= zapcore.ErrorLevel {
			t.Fatalf("ErrRecordNotFound 不应记录为错误，实际 %v", entry)
		}
	}
}

func TestGormLoggerLogsExecutionError(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	err := gorm.ErrInvalidDB

	newGormLogger(zap.New(core), "info", time.Second).Trace(context.Background(),
		time.Now(),
		func() (string, int64) { return "SELECT 1", 0 },
		err,
	)

	entries := logs.All()
	if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
		t.Fatalf("SQL 执行失败应以 error 级别记录，实际 %+v", entries)
	}
}

func TestGormLoggerLogMode(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)

	base := newGormLogger(zap.New(core), "silent", time.Second)
	silent := base.LogMode(gormlogger.Silent)

	if silent == base {
		t.Fatal("LogMode 应返回新的实例")
	}

	silent.Trace(context.Background(), time.Now(),
		func() (string, int64) { return "SELECT 1", 1 }, nil)

	if len(logs.All()) != 0 {
		t.Fatalf("silent 级别不应输出日志，实际 %d 条", len(logs.All()))
	}
}
