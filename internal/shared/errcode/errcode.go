package errcode

import (
	"errors"
	"fmt"
	"net/http"
)

// 业务错误码分段：1xxxx 通用，2xxxx 鉴权，3xxxx 词库，4xxxx 学习，5xxxx 服务端。
const (
	CodeOK              = 0
	CodeInvalidParams   = 10001
	CodeNotFound        = 10002
	CodeConflict        = 10003
	CodeTooManyRequests = 10004
	CodeInternal        = 10005

	CodeUnauthorized = 20001
	CodeForbidden    = 20002
)

// Error 是贯穿业务层的统一错误类型。
// Code 面向调用方，Status 决定 HTTP 状态码，Err 仅用于服务端日志，不对外暴露。
type Error struct {
	Status  int
	Code    int
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Is / errors.As 透传内部原因。
func (e *Error) Unwrap() error { return e.Err }

// WithCause 返回附带内部原因的错误副本，用于记录日志而对外保持原提示。
func (e *Error) WithCause(err error) *Error {
	clone := *e
	clone.Err = err
	return &clone
}

// WithMessage 返回替换了对外提示的错误副本。
func (e *Error) WithMessage(message string) *Error {
	clone := *e
	clone.Message = message
	return &clone
}

// New 构造一个业务错误。
func New(status, code int, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// From 从任意 error 中提取业务错误，非业务错误统一归一为内部错误。
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var target *Error
	if errors.As(err, &target) {
		return target
	}
	return ErrInternal.WithCause(err)
}

// 预定义的通用业务错误。
var (
	ErrInvalidParams   = New(http.StatusBadRequest, CodeInvalidParams, "请求参数不合法")
	ErrNotFound        = New(http.StatusNotFound, CodeNotFound, "资源不存在")
	ErrConflict        = New(http.StatusConflict, CodeConflict, "资源已存在或状态冲突")
	ErrTooManyRequests = New(http.StatusTooManyRequests, CodeTooManyRequests, "请求过于频繁")
	ErrInternal        = New(http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	ErrUnauthorized    = New(http.StatusUnauthorized, CodeUnauthorized, "未登录或登录状态已失效")
	ErrForbidden       = New(http.StatusForbidden, CodeForbidden, "没有操作权限")
)