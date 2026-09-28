package middleware

import (
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kotoba/internal/shared/errcode"
	"kotoba/internal/shared/response"
)

// Recovery 捕获 panic，记录堆栈后返回统一的 500 响应，避免单个请求拖垮进程。
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}

			fields := []zap.Field{
				zap.Any("panic", rec),
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.ByteString("stack", debug.Stack()),
			}
			if id := response.RequestID(c); id != "" {
				fields = append(fields, zap.String("request_id", id))
			}

			log.Error("请求处理发生 panic", fields...)
			response.AbortFail(c, errcode.ErrInternal)
		}()

		c.Next()
	}
}
