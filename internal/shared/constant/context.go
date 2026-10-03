package constant

// 请求上下文与响应头相关的键名。
const (
	// HeaderRequestID 链路 ID 的响应头名称。
	HeaderRequestID = "X-Request-ID"

	// ContextKeyRequestID gin.Context 中存放链路 ID 的键。
	ContextKeyRequestID = "request_id"

	// FieldRequestID 结构化日志中链路 ID 的字段名，与响应头保持一致便于检索。
	FieldRequestID = "request_id"
)
