package validator

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	govalidator "github.com/go-playground/validator/v10"

	"kotoba/internal/shared/errcode"
)

type sampleRequest struct {
	Message  string `json:"message" validate:"required"`
	Nickname string `json:"nickname" validate:"min=2,max=8"`
	Email    string `json:"email" validate:"email"`
	Role     string `json:"role" validate:"oneof=admin user"`
	Code     string `json:"code" validate:"uuid"`
}

// newValidator 复刻 gin 的配置：注册 json tag 作为字段名。
func newValidator() *govalidator.Validate {
	v := govalidator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return v
}

func TestTranslateProducesFriendlyMessages(t *testing.T) {
	v := newValidator()

	cases := []struct {
		name    string
		req     sampleRequest
		wantMsg string
	}{
		{
			name:    "必填项缺失",
			req:     sampleRequest{Nickname: "ok", Email: "a@b.com", Role: "admin"},
			wantMsg: "message 为必填项",
		},
		{
			name:    "长度越界",
			req:     sampleRequest{Message: "hi", Nickname: "x", Email: "a@b.com", Role: "admin"},
			wantMsg: "nickname 不能小于 2",
		},
		{
			name:    "邮箱格式",
			req:     sampleRequest{Message: "hi", Nickname: "ok", Email: "not-an-email", Role: "admin"},
			wantMsg: "email 格式不正确",
		},
		{
			name:    "枚举取值",
			req:     sampleRequest{Message: "hi", Nickname: "ok", Email: "a@b.com", Role: "guest"},
			wantMsg: "role 的取值必须是 [admin user] 之一",
		},
		{
			name:    "未映射的规则走兜底提示",
			req:     sampleRequest{Message: "hi", Nickname: "ok", Email: "a@b.com", Role: "admin", Code: "x"},
			wantMsg: "code 参数不合法",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Struct(tc.req)
			if err == nil {
				t.Fatal("期望校验失败，实际通过")
			}

			translated := Translate(err)
			if translated.Status != 400 || translated.Code != errcode.CodeInvalidParams {
				t.Fatalf("应归一为参数不合法错误，实际 status=%d code=%d", translated.Status, translated.Code)
			}
			if !strings.Contains(translated.Message, tc.wantMsg) {
				t.Fatalf("提示信息应包含 %q，实际 %q", tc.wantMsg, translated.Message)
			}
		})
	}
}

func TestTranslateJoinsMultipleErrors(t *testing.T) {
	v := newValidator()

	err := v.Struct(sampleRequest{})
	if err == nil {
		t.Fatal("期望校验失败，实际通过")
	}

	translated := Translate(err)
	if !strings.Contains(translated.Message, "；") {
		t.Fatalf("多个字段同时出错时应拼接提示，实际 %q", translated.Message)
	}
}

func TestTranslateNonValidationError(t *testing.T) {
	cause := errors.New("unexpected end of JSON input")

	translated := Translate(cause)
	if translated.Message != "请求参数格式不正确" {
		t.Fatalf("非校验错误应给出兜底提示，实际 %q", translated.Message)
	}
	// 原始原因保留给日志，不进入响应体。
	if !errors.Is(translated, cause) {
		t.Fatal("原始原因应可通过 errors.Is 追溯")
	}
}

func TestTranslateNil(t *testing.T) {
	if Translate(nil) != nil {
		t.Fatal("Translate(nil) 应返回 nil")
	}
}
