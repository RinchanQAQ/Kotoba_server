package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"kotoba/internal/shared/constant"
	"kotoba/internal/shared/errcode"
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

// RequestID 读取当前请求的链路 ID。
func RequestID(c *gin.Context) string {
	if v, ok := c.Get(constant.ContextKeyRequestID); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}