package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kotoba/internal/shared/response"
)

// AccessLog 记录每个请求的访问日志，按状态码选择日志级别。
func AccessLog(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

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
		if id := response.RequestID(c); id != "" {
			fields = append(fields, zap.String("request_id", id))
		}
		if c.Errors != nil && len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()))
		}

		switch status := c.Writer.Status(); {
		case status >= 500:
			log.Error("请求处理失败", fields...)
		case status >= 400:
			log.Warn("请求被拒绝", fields...)
		default:
			log.Info("请求完成", fields...)
		}
	}
}

