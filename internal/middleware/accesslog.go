package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kotoba/internal/shared/logging"
)

// AccessLog 记录每个请求的访问日志，并按状态码选择日志级别。
// request_id 由 RequestLogger 注入 context，这里自动带上，无需手动拼字段。
func AccessLog(log *zap.Logger) gin.HandlerFunc {
	if log == nil {
		log = zap.NewNop()
	}

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		reqLog := logging.FromOr(c.Request.Context(), log)

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
			zap.Int("size", c.Writer.Size()),
		}
		if query != "" {
			fields = append(fields, zap.String("query", query))
		}
		if c.Errors != nil && len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()))
		}

		switch status := c.Writer.Status(); {
		case status >= 500:
			reqLog.Error("请求处理失败", fields...)
		case status >= 400:
			reqLog.Warn("请求被拒绝", fields...)
		default:
			reqLog.Info("请求完成", fields...)
		}
	}
}
