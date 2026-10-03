package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"kotoba/internal/bootstrap"
	"kotoba/internal/shared/constant"
	"kotoba/internal/shared/errcode"
	"kotoba/internal/shared/response"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// newTestApp 构造只依赖配置与日志的 App（探针之外的用例不会触达 MySQL/Redis）。
func newTestApp(t *testing.T) (*bootstrap.App, *observer.ObservedLogs) {
	t.Helper()

	core, logs := observer.New(zapcore.DebugLevel)

	cfg := &bootstrap.Config{
		Env: bootstrap.EnvTest,
		App: bootstrap.AppConfig{
			Name: "kotoba",
			Env:  bootstrap.EnvTest,
			Addr: ":8080",
			CORS: bootstrap.CORSConfig{AllowOrigins: []string{"http://localhost:5173"}},
		},
	}

	return &bootstrap.App{Config: cfg, Logger: zap.New(core)}, logs
}

func do(t *testing.T, r *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) response.Body {
	t.Helper()

	var body response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体不是统一格式: %v (%s)", err, w.Body.String())
	}
	return body
}

func TestHealthz(t *testing.T) {
	application, _ := newTestApp(t)

	w := do(t, NewRouter(application), http.MethodGet, "/healthz", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("存活探针应返回 200，实际 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok"`) {
		t.Fatalf("存活探针响应异常: %s", w.Body.String())
	}
}

func TestPingUsesUnifiedResponse(t *testing.T) {
	application, _ := newTestApp(t)

	w := do(t, NewRouter(application), http.MethodGet, "/v1/ping", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("业务接口应返回 200，实际 %d", w.Code)
	}

	body := decodeBody(t, w)
	if body.Code != errcode.CodeOK {
		t.Fatalf("业务响应码应为 %d，实际 %d", errcode.CodeOK, body.Code)
	}
}

func TestNoRouteReturnsUnifiedBody(t *testing.T) {
	application, _ := newTestApp(t)

	w := do(t, NewRouter(application), http.MethodGet, "/v1/not-exist", "", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("未匹配路由应返回 404，实际 %d", w.Code)
	}

	body := decodeBody(t, w)
	if body.Code != errcode.CodeNotFound {
		t.Fatalf("兜底 404 应使用统一业务码 %d，实际 %d", errcode.CodeNotFound, body.Code)
	}
	// 兜底路由也要带链路 ID，方便定位问题。
	if body.RequestID == "" {
		t.Fatal("兜底 404 响应应包含 request_id")
	}
}

func TestNoMethodReturnsUnifiedBody(t *testing.T) {
	application, _ := newTestApp(t)

	w := do(t, NewRouter(application), http.MethodPost, "/healthz", "", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("方法不匹配应返回 405，实际 %d", w.Code)
	}

	if body := decodeBody(t, w); body.Code != errcode.CodeMethodNotAllowed {
		t.Fatalf("兜底 405 应使用统一业务码 %d，实际 %d", errcode.CodeMethodNotAllowed, body.Code)
	}
}

func TestEchoSuccessPropagatesRequestID(t *testing.T) {
	application, logs := newTestApp(t)

	w := do(t, NewRouter(application), http.MethodPost, "/v1/echo",
		`{"message":"hello"}`, map[string]string{constant.HeaderRequestID: "trace-abc"})

	if w.Code != http.StatusOK {
		t.Fatalf("合法请求应返回 200，实际 %d (%s)", w.Code, w.Body.String())
	}
	if got := w.Header().Get(constant.HeaderRequestID); got != "trace-abc" {
		t.Fatalf("响应头应回写链路 ID，实际 %q", got)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析响应体失败: %v", err)
	}
	if payload.Data.Message != "hello" || payload.Data.RequestID != "trace-abc" {
		t.Fatalf("响应数据不符合预期: %+v", payload.Data)
	}

	// handler 内部的业务日志应自动带上同一个 request_id。
	var found bool
	for _, entry := range logs.All() {
		if entry.Message != "收到 echo 请求" {
			continue
		}
		found = true
		if got := entry.ContextMap()[constant.FieldRequestID]; got != "trace-abc" {
			t.Fatalf("业务日志的 request_id 应为 trace-abc，实际 %v", got)
		}
	}
	if !found {
		t.Fatal("未记录到 echo 业务日志")
	}
}

func TestEchoValidationFailureReturnsUnifiedBody(t *testing.T) {
	application, logs := newTestApp(t)

	w := do(t, NewRouter(application), http.MethodPost, "/v1/echo", `{"message":""}`, nil)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("参数不合法应返回 400，实际 %d", w.Code)
	}

	body := decodeBody(t, w)
	if body.Code != errcode.CodeInvalidParams {
		t.Fatalf("业务错误码应为 %d，实际 %d", errcode.CodeInvalidParams, body.Code)
	}
	if !strings.Contains(body.Message, "message 为必填项") {
		t.Fatalf("应返回翻译后的中文提示，实际 %q", body.Message)
	}

	// 原始校验错误进入日志，便于排查；但不出现在响应体中。
	var logged bool
	for _, entry := range logs.All() {
		if raw, ok := entry.ContextMap()["errors"].(string); ok && strings.Contains(raw, "required") {
			logged = true
		}
	}
	if !logged {
		t.Fatal("原始校验错误应被访问日志记录")
	}
	if strings.Contains(w.Body.String(), "required") {
		t.Fatalf("响应体不应暴露校验器原始信息: %s", w.Body.String())
	}
}

func TestPanicIsRecoveredIntoUnifiedResponse(t *testing.T) {
	application, logs := newTestApp(t)

	r := NewRouter(application)
	r.GET("/v1/panic", func(c *gin.Context) { panic("boom") })

	w := do(t, r, http.MethodGet, "/v1/panic", "", map[string]string{constant.HeaderRequestID: "panic-trace"})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic 应返回 500，实际 %d", w.Code)
	}

	body := decodeBody(t, w)
	if body.Code != errcode.CodeInternal || body.RequestID != "panic-trace" {
		t.Fatalf("panic 响应体不符合预期: %+v", body)
	}

	var hasStack bool
	for _, entry := range logs.All() {
		if _, ok := entry.ContextMap()["stack"]; ok {
			hasStack = true
		}
	}
	if !hasStack {
		t.Fatal("panic 应被记录堆栈")
	}
}
