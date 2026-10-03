// Package logging 负责把「请求级 logger」沿 context 向下传递，
// 让 handler、service、repository、gorm 打印的日志自动带上同一个 request_id，
// 从而把一次请求产生的所有日志串联起来。
package logging

import (
	"context"

	"go.uber.org/zap"
)

// contextKey 是私有的 context key 类型，避免与其他包的 key 冲突。
type contextKey struct{}

var loggerKey contextKey

// WithLogger 把请求级 logger 注入 context。
func WithLogger(ctx context.Context, log *zap.Logger) context.Context {
	if ctx == nil || log == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerKey, log)
}

// From 取回请求级 logger；未注入时返回 no-op logger，
// 调用方无需判空，业务代码可以放心地 logging.From(ctx).Info(...)。
func From(ctx context.Context) *zap.Logger {
	if log, ok := lookup(ctx); ok {
		return log
	}
	return zap.NewNop()
}

// FromOr 取回请求级 logger；未注入时回落到 fallback（fallback 也为空时返回 no-op logger）。
// 适用于 gorm 这类可能拿不到请求 context 的底层组件。
func FromOr(ctx context.Context, fallback *zap.Logger) *zap.Logger {
	if log, ok := lookup(ctx); ok {
		return log
	}
	if fallback != nil {
		return fallback
	}
	return zap.NewNop()
}

func lookup(ctx context.Context) (*zap.Logger, bool) {
	if ctx == nil {
		return nil, false
	}
	log, ok := ctx.Value(loggerKey).(*zap.Logger)
	if !ok || log == nil {
		return nil, false
	}
	return log, true
}
