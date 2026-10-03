// Package validator 把 go-playground/validator 的校验错误翻译成
// 面向调用方的中文提示，并包装成统一的业务错误（errcode.Error）。
//
// 包初始化时会把 gin 默认校验器的字段名换成 json tag，
// 因此提示里出现的是接口文档中的字段名（如 message 而非 Message）。
package validator

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin/binding"
	govalidator "github.com/go-playground/validator/v10"

	"kotoba/internal/shared/errcode"
)

// 包加载即注册字段名解析规则，避免调用方忘记注册导致提示字段名不一致。
func init() {
	RegisterJSONFieldNames()
}

// RegisterJSONFieldNames 让校验错误提示使用 json tag 作为字段名。
//
// gin 默认用 Go 字段名（Message），而调用方看到的是 json 字段（message），
// 因此在校由 NewRouter 启动时注册一次，保证提示与接口文档一致。
// 重复调用安全。
func RegisterJSONFieldNames() {
	engine, ok := binding.Validator.Engine().(*govalidator.Validate)
	if !ok {
		return
	}
	engine.RegisterTagNameFunc(jsonFieldName)
}

// jsonFieldName 提取结构体字段的 json 名，缺失时退回 Go 字段名。
func jsonFieldName(field reflect.StructField) string {
	name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
	if name == "" || name == "-" {
		return field.Name
	}
	return name
}

// tagMessages 校验规则 -> 提示模板；%s 依次为字段名、规则参数。
var tagMessages = map[string]string{
	"required":   "%s 为必填项",
	"email":      "%s 格式不正确",
	"url":        "%s 必须是合法的 URL",
	"numeric":    "%s 必须是数字",
	"alphanum":   "%s 只能包含字母和数字",
	"min":        "%s 不能小于 %s",
	"max":        "%s 不能大于 %s",
	"len":        "%s 长度必须为 %s",
	"gte":        "%s 不能小于 %s",
	"lte":        "%s 不能大于 %s",
	"gt":         "%s 必须大于 %s",
	"lt":         "%s 必须小于 %s",
	"oneof":      "%s 的取值必须是 [%s] 之一",
	"startswith": "%s 必须以 %s 开头",
	"endswith":   "%s 必须以 %s 结尾",
	"datetime":   "%s 时间格式应为 %s",
	"eqfield":    "%s 必须与 %s 保持一致",
}

// 无需规则参数的标签，拼提示时只传字段名。
var noParamTags = map[string]struct{}{
	"required": {}, "email": {}, "url": {}, "numeric": {}, "alphanum": {},
}

// Translate 把绑定/校验失败的错误翻译为「参数不合法」业务错误。
//
// 传入 nil 返回 nil；校验规则未覆盖时给出兜底提示；
// 非校验类错误（如 JSON 语法错误）同样归为参数不合法，并保留原始原因供日志排查。
func Translate(err error) *errcode.Error {
	if err == nil {
		return nil
	}

	var verrs govalidator.ValidationErrors
	if errors.As(err, &verrs) && len(verrs) > 0 {
		messages := make([]string, 0, len(verrs))
		for _, fe := range verrs {
			messages = append(messages, message(fe))
		}
		return errcode.ErrInvalidParams.WithMessage(strings.Join(messages, "；"))
	}

	return errcode.ErrInvalidParams.WithMessage("请求参数格式不正确").WithCause(err)
}

func message(fe govalidator.FieldError) string {
	field := fe.Field()
	if field == "" {
		field = fe.StructField()
	}

	tmpl, ok := tagMessages[fe.Tag()]
	if !ok {
		return fmt.Sprintf("%s 参数不合法", field)
	}

	if _, ok := noParamTags[fe.Tag()]; ok {
		return fmt.Sprintf(tmpl, field)
	}
	return fmt.Sprintf(tmpl, field, fe.Param())
}
