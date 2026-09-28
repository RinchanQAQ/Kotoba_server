package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"

	"kotoba/internal/shared/constant"
)

// RequestID 为每个请求分配链路 ID，写入上下文并回写到响应头。
// 若调用方已带上 X-Request-ID，则沿用该值以便跨服务串联。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(constant.HeaderRequestID)
		if id == "" {
			id = newRequestID()
		}
		c.Set(constant.ContextKeyRequestID, id)
		c.Writer.Header().Set(constant.HeaderRequestID, id)
		c.Next()
	}
}

// newRequestID 生成 UUID v4 形态的十六进制字符串。
func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	s := hex.EncodeToString(b)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}
