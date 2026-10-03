package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"kotoba/internal/shared/constant"
	"kotoba/internal/shared/errcode"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// newEngine 构造一个带链路 ID 的最小引擎，用于驱动响应体断言。
func newEngine(handler gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(constant.ContextKeyRequestID, "req-1")
	})
	r.GET("/", handler)
	r.POST("/", handler)
	return r
}

func decode(t *testing.T, w *httptest.ResponseRecorder) Body {
	t.Helper()

	var body Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体不是统一格式: %v (%s)", err, w.Body.String())
	}
	return body
}

func TestOK(t *testing.T) {
	r := newEngine(func(c *gin.Context) { OK(c, gin.H{"id": 7}) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("状态码应为 200，实际 %d", w.Code)
	}

	body := decode(t, w)
	if body.Code != errcode.CodeOK || body.Message != "ok" {
		t.Fatalf("成功响应体不符合约定: %+v", body)
	}
	if body.RequestID != "req-1" {
		t.Fatalf("成功响应应回带链路 ID，实际 %q", body.RequestID)
	}
}

func TestFailWithBusinessError(t *testing.T) {
	r := newEngine(func(c *gin.Context) { Fail(c, errcode.ErrNotFound) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("业务错误应决定 HTTP 状态码，实际 %d", w.Code)
	}

	body := decode(t, w)
	if body.Code != errcode.CodeNotFound || body.Message != errcode.ErrNotFound.Message {
		t.Fatalf("业务错误响应体不符合约定: %+v", body)
	}
}

func TestFailHidesInternalErrorDetails(t *testing.T) {
	cause := errors.New("dial tcp 127.0.0.1:3306: connect: connection refused")
	r := newEngine(func(c *gin.Context) { Fail(c, cause) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("非业务错误应归一为 500，实际 %d", w.Code)
	}

	// 对外只暴露通用提示，底层原因不进入响应体。
	if strings.Contains(w.Body.String(), "connection refused") {
		t.Fatalf("响应体不应泄漏内部错误细节: %s", w.Body.String())
	}

	body := decode(t, w)
	if body.Code != errcode.CodeInternal || body.Message != "服务器内部错误" {
		t.Fatalf("内部错误响应体不符合约定: %+v", body)
	}

	// 但底层原因必须保留在错误链上，供日志排查。
	if !errors.Is(errcode.From(cause), cause) {
		t.Fatal("内部错误原因应保留在错误链上")
	}
}

func TestAbortFailStopsRemainingHandlers(t *testing.T) {
	reached := false

	// gin 的 Abort 只阻止后续 handler，不会中断当前函数，因此用两个 handler 验证。
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(constant.ContextKeyRequestID, "req-1") })
	r.GET("/", func(c *gin.Context) { AbortFail(c, errcode.ErrUnauthorized) },
		func(c *gin.Context) { reached = true })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if reached {
		t.Fatal("AbortFail 之后不应执行后续 handler")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码应为 401，实际 %d", w.Code)
	}
	if body := decode(t, w); body.Code != errcode.CodeUnauthorized {
		t.Fatalf("业务错误码应为 %d，实际 %d", errcode.CodeUnauthorized, body.Code)
	}
}

func TestBodyOmitsEmptyData(t *testing.T) {
	r := newEngine(func(c *gin.Context) { Fail(c, errcode.ErrNotFound) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	// data 字段为空时不应出现，避免调用方误判。
	if strings.Contains(w.Body.String(), `"data"`) {
		t.Fatalf("错误响应不应包含 data 字段: %s", w.Body.String())
	}
}

func TestNoContent(t *testing.T) {
	r := newEngine(func(c *gin.Context) { NoContent(c) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("204 响应不应有响应体，实际 code=%d body=%q", w.Code, w.Body.String())
	}
}

type bindRequest struct {
	Message string `json:"message" binding:"required,min=1,max=200"`
}

func TestBindJSON(t *testing.T) {
	r := newEngine(func(c *gin.Context) {
		var req bindRequest
		if !BindJSON(c, &req) {
			return
		}
		OK(c, gin.H{"message": req.Message})
	})

	t.Run("校验通过", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":"hi"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("合法请求应返回 200，实际 %d (%s)", w.Code, w.Body.String())
		}
	})

	t.Run("校验失败返回统一格式", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":""}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("参数不合法应返回 400，实际 %d", w.Code)
		}

		body := decode(t, w)
		if body.Code != errcode.CodeInvalidParams {
			t.Fatalf("业务错误码应为 %d，实际 %d", errcode.CodeInvalidParams, body.Code)
		}
		if !strings.Contains(body.Message, "message 为必填项") {
			t.Fatalf("提示信息应命中字段翻译，实际 %q", body.Message)
		}
	})

	t.Run("JSON 语法错误返回统一格式", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("请求体格式错误应返回 400，实际 %d", w.Code)
		}
		if body := decode(t, w); body.Code != errcode.CodeInvalidParams {
			t.Fatalf("业务错误码应为 %d，实际 %d", errcode.CodeInvalidParams, body.Code)
		}
	})
}

func TestRequestIDMissing(t *testing.T) {
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		if id := RequestID(c); id != "" {
			t.Fatalf("未设置链路 ID 时应返回空串，实际 %q", id)
		}
		c.Status(http.StatusNoContent)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
