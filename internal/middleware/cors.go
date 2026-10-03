package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS 处理跨域请求。
// allowOrigins 为空时回显任意来源，仅建议本地开发使用；生产环境必须显式配置白名单。
func CORS(allowOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowOrigins))
	for _, origin := range allowOrigins {
		if origin = strings.TrimSpace(origin); origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	allowAll := len(allowed) == 0

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if origin != "" {
			_, inWhitelist := allowed[origin]
			switch {
			case allowAll:
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			case inWhitelist:
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Add("Vary", "Origin")
			case c.Request.Method == http.MethodOptions:
				// 非白名单来源的预检请求直接拒绝。
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}

		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(12*60*60))
		if !allowAll {
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
