package middleware

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kotoba/internal/shared/constant"
	"kotoba/internal/shared/logging"
	"kotoba/internal/shared/response"
)

// RequestLogger 把带 request_id 字段的 logger 注入请求 context。
//
// 必须排在 RequestID() 之后：RequestID 负责生成/透传链路 ID，
// RequestLogger 负责基于该 ID 派生日志实例。
// 之后任意一层只要拿到 ctx，就可以用 logging.From(ctx) 输出可串联的日志。
func RequestLogger(base *zap.Logger) gin.HandlerFunc {
	if base == nil {
		base = zap.NewNop()
	}

	return func(c *gin.Context) {
		reqLog := base
		if id := response.RequestID(c); id != "" {
			reqLog = base.With(zap.String(constant.FieldRequestID, id))
		}

		ctx := logging.WithLogger(c.Request.Context(), reqLog)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
