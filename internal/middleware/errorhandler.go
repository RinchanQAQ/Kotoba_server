package middleware

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kotoba/internal/shared/errcode"
	"kotoba/internal/shared/logging"
	"kotoba/internal/shared/response"
)

// ErrorHandler 统一收敛 handler 上报的错误。
//
// 业务代码只需要 c.Error(err) 后 return，无需关心 HTTP 状态码与响应体：
// 该中间件会在 handler 结束后取出最后一个错误，按业务错误码写成统一响应体。
// 若 handler 已经自行写回了响应，则不重复覆盖。
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}

		err := c.Errors.Last().Err
		e := errcode.From(err)

		// request_id 由 RequestLogger 注入，这里无需再手动拼字段。
		log := logging.From(c.Request.Context())
		if e.Status >= 500 {
			log.Error("请求处理失败", zap.Int("code", e.Code), zap.Error(err))
		} else {
			log.Warn("请求被拒绝", zap.Int("code", e.Code), zap.Error(err))
		}

		response.Fail(c, err)
	}
}
