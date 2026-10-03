package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"kotoba/internal/shared/constant"
	"kotoba/internal/shared/errcode"
	"kotoba/internal/shared/logging"
	"kotoba/internal/shared/response"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// newEngine 构造只挂载指定中间件的引擎。
func newEngine(handlers ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(handlers...)
	return r
}

// observedLogger 返回可断言的 logger。
func observedLogger(level zapcore.Level) (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(level)
	return zap.New(core), logs
}

func TestRequestIDEchoesInboundID(t *testing.T) {
	r := newEngine(RequestID())
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, response.RequestID(c)) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(constant.HeaderRequestID, "trace-123")
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "trace-123" {
		t.Fatalf("应沿用调用方传入的链路 ID，实际 %q", got)
	}
	if got := w.Header().Get(constant.HeaderRequestID); got != "trace-123" {
		t.Fatalf("响应头应回写链路 ID，实际 %q", got)
	}
}

func TestRequestIDGeneratesWhenAbsent(t *testing.T) {
	r := newEngine(RequestID())
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, response.RequestID(c)) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	id := w.Body.String()
	if len(id) != 36 {
		t.Fatalf("未携带链路 ID 时应生成 UUID 形态的 ID，实际 %q", id)
	}
	if w.Header().Get(constant.HeaderRequestID) != id {
		t.Fatal("响应头中的链路 ID 应与上下文一致")
	}
}

func TestRequestLoggerInjectsRequestScopedLogger(t *testing.T) {
	logger, logs := observedLogger(zapcore.DebugLevel)

	r := newEngine(RequestID(), RequestLogger(logger))
	r.GET("/", func(c *gin.Context) {
		logging.From(c.Request.Context()).Info("handler log")
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(constant.HeaderRequestID, "abc")
	r.ServeHTTP(w, req)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("应记录 1 条业务日志，实际 %d", len(entries))
	}
	if got := entries[0].ContextMap()[constant.FieldRequestID]; got != "abc" {
		t.Fatalf("业务日志应自动带上 request_id，实际 %v", got)
	}
}

func TestRecoveryReturnsUnifiedResponse(t *testing.T) {
	logger, logs := observedLogger(zapcore.ErrorLevel)

	r := newEngine(RequestID(), RequestLogger(logger), Recovery(logger))
	r.GET("/panic", func(c *gin.Context) { panic("boom") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set(constant.HeaderRequestID, "panic-id")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic 应返回 500，实际 %d", w.Code)
	}

	var body response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体不是统一格式: %v", err)
	}
	if body.Code != errcode.CodeInternal || body.Message != "服务器内部错误" {
		t.Fatalf("panic 响应体不符合预期: %+v", body)
	}
	if body.RequestID != "panic-id" {
		t.Fatalf("panic 响应应回带链路 ID，实际 %q", body.RequestID)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("应记录 1 条 panic 日志，实际 %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["panic"] != "boom" {
		t.Fatalf("日志应包含 panic 值，实际 %v", fields["panic"])
	}
	if stack, ok := fields["stack"].(string); !ok || !strings.Contains(stack, "TestRecoveryReturnsUnifiedResponse") {
		t.Fatal("日志应包含 panic 堆栈")
	}
	if fields[constant.FieldRequestID] != "panic-id" {
		t.Fatalf("panic 日志应带 request_id，实际 %v", fields[constant.FieldRequestID])
	}
}

func TestCORSAllowsWhitelistedOrigin(t *testing.T) {
	r := newEngine(CORS([]string{"http://localhost:5173"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("白名单来源应被放行，实际 %q", got)
	}
	if !strings.Contains(w.Header().Get("Vary"), "Origin") {
		t.Fatal("命中白名单时应追加 Vary: Origin")
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("固定白名单场景应允许携带凭证")
	}
}

func TestCORSRejectsPreflightFromUnknownOrigin(t *testing.T) {
	r := newEngine(CORS([]string{"http://localhost:5173"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://evil.example")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("非白名单来源的预检应被拒绝，实际 %d", w.Code)
	}
}

func TestCORSAnswersPreflight(t *testing.T) {
	r := newEngine(CORS([]string{"http://localhost:5173"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("合法预检应返回 204，实际 %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "POST") {
		t.Fatal("预检响应应声明允许的方法")
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), constant.HeaderRequestID) {
		t.Fatal("预检响应应放行 X-Request-ID 头")
	}
}

func TestCORSAllowsAnyOriginWhenWhitelistEmpty(t *testing.T) {
	// 空白名单＝回显任意来源，仅供本地开发；生产环境由配置校验拦住。
	r := newEngine(CORS(nil))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://any.example")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://any.example" {
		t.Fatalf("空白名单应回显来源，实际 %q", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("回显任意来源时不应声明允许凭证")
	}
}

func TestErrorHandlerConvertsContextErrors(t *testing.T) {
	logger, logs := observedLogger(zapcore.DebugLevel)

	r := newEngine(RequestID(), RequestLogger(logger), ErrorHandler())
	r.GET("/", func(c *gin.Context) { _ = c.Error(errcode.ErrConflict) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(constant.HeaderRequestID, "err-id")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("业务错误应映射为 409，实际 %d", w.Code)
	}

	var body response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体不是统一格式: %v", err)
	}
	if body.Code != errcode.CodeConflict {
		t.Fatalf("业务错误码应为 %d，实际 %d", errcode.CodeConflict, body.Code)
	}

	entries := logs.All()
	if len(entries) != 1 || entries[0].Level != zapcore.WarnLevel {
		t.Fatalf("4xx 业务错误应以 warn 记录，实际 %+v", entries)
	}
	if entries[0].ContextMap()[constant.FieldRequestID] != "err-id" {
		t.Fatal("错误日志应带 request_id")
	}
}

func TestErrorHandlerKeepsResponseAlreadyWritten(t *testing.T) {
	r := newEngine(RequestID(), ErrorHandler())
	r.GET("/", func(c *gin.Context) {
		response.Fail(c, errcode.ErrNotFound)
		_ = c.Error(errors.New("该错误不应覆盖已写回的响应"))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("已写回的响应不应被覆盖，实际 %d", w.Code)
	}

	var body response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体不是统一格式: %v", err)
	}
	if body.Code != errcode.CodeNotFound {
		t.Fatalf("业务错误码应为 %d，实际 %d", errcode.CodeNotFound, body.Code)
	}
}

func TestAccessLogRecordsResult(t *testing.T) {
	logger, logs := observedLogger(zapcore.DebugLevel)

	r := newEngine(RequestID(), RequestLogger(logger), AccessLog(logger))
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok?a=1", nil)
	req.Header.Set(constant.HeaderRequestID, "access-id")
	r.ServeHTTP(w, req)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("应记录 1 条访问日志，实际 %d", len(entries))
	}

	fields := entries[0].ContextMap()
	if fields["path"] != "/ok" || fields["query"] != "a=1" {
		t.Fatalf("访问日志应包含路径与查询串，实际 %v", fields)
	}
	if fields["status"] != int64(http.StatusOK) {
		t.Fatalf("访问日志应包含状态码，实际 %v", fields["status"])
	}
	if fields[constant.FieldRequestID] != "access-id" {
		t.Fatal("访问日志应带 request_id")
	}
}

func TestAccessLogUsesErrorLevelFor5xx(t *testing.T) {
	logger, logs := observedLogger(zapcore.DebugLevel)

	r := newEngine(RequestID(), RequestLogger(logger), AccessLog(logger))
	r.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	entries := logs.All()
	if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
		t.Fatalf("5xx 应以 error 级别记录，实际 %+v", entries)
	}
}
