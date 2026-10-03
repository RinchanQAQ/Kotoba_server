package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kotoba/internal/shared/constant"
	"kotoba/internal/shared/errcode"
	"kotoba/internal/shared/validator"
)

// Body 是所有业务接口统一返回的响应结构。
type Body struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// OK 返回 200 与业务数据。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: errcode.CodeOK, Message: "ok", Data: data, RequestID: RequestID(c)})
}

// Created 返回 201 与新建的资源。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Body{Code: errcode.CodeOK, Message: "ok", Data: data, RequestID: RequestID(c)})
}

// NoContent 返回 204，适用于无响应体的写操作。
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Fail 按业务错误写入响应；非业务错误会被归一为内部错误。
func Fail(c *gin.Context, err error) {
	e := errcode.From(err)
	c.JSON(e.Status, Body{Code: e.Code, Message: e.Message, RequestID: RequestID(c)})
}

// AbortFail 写入错误响应并终止后续 handler，供中间件与鉴权逻辑使用。
func AbortFail(c *gin.Context, err error) {
	e := errcode.From(err)
	c.AbortWithStatusJSON(e.Status, Body{Code: e.Code, Message: e.Message, RequestID: RequestID(c)})
}

// BindJSON 解析并校验 JSON 请求体。
// 校验失败时已经写好统一错误响应，调用方直接 return 即可：
//
//	var req CreateUserRequest
//	if !response.BindJSON(c, &req) {
//	    return
//	}
func BindJSON(c *gin.Context, obj any) bool {
	return bind(c, obj, func() error { return c.ShouldBindJSON(obj) })
}

// BindQuery 解析并校验 URL query 参数。
func BindQuery(c *gin.Context, obj any) bool {
	return bind(c, obj, func() error { return c.ShouldBindQuery(obj) })
}

// BindURI 解析并校验路径参数。
func BindURI(c *gin.Context, obj any) bool {
	return bind(c, obj, func() error { return c.ShouldBindUri(obj) })
}

func bind(c *gin.Context, obj any, do func() error) bool {
	err := do()
	if err == nil {
		return true
	}

	// 原始错误挂到 c.Errors 供日志排查（ErrorHandler 不会重复写响应），
	// 对外只返回翻译后的中文提示。
	c.Error(err)
	Fail(c, validator.Translate(err))
	return false
}

// RequestID 读取当前请求的链路 ID。
func RequestID(c *gin.Context) string {
	if v, ok := c.Get(constant.ContextKeyRequestID); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}
