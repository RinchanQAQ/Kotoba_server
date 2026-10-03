package middleware

import (
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kotoba/internal/shared/errcode"
	"kotoba/internal/shared/logging"
	"kotoba/internal/shared/response"
)

// Recovery 捕获 panic，记录堆栈后返回统一的 500 响应，避免单个请求拖垮进程。
func Recovery(log *zap.Logger) gin.HandlerFunc {
	if log == nil {
		log = zap.NewNop()
	}

	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}

			// 带上 request_id，方便用链路 ID 直接检索到这一次崩溃的完整日志。
			logging.FromOr(c.Request.Context(), log).Error("请求处理发生 panic",
				zap.Any("panic", rec),
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.ByteString("stack", debug.Stack()),
			)

			response.AbortFail(c, errcode.ErrInternal)
		}()

		c.Next()
	}
}
