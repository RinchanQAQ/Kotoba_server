package bootstrap

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger 依据配置创建日志实例。
func NewLogger(cfg LogConfig, appName string) (*zap.Logger, error) {
	level, err := parseLogLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
	encCfg.EncodeDuration = zapcore.MillisDurationEncoder

	var encoder zapcore.Encoder
	if strings.EqualFold(cfg.Format, "json") {
		encoder = zapcore.NewJSONEncoder(encCfg)
	} else {
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	}

	core := zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), level)

	return zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.Fields(zap.String("app", appName)),
	), nil
}

func parseLogLevel(s string) (zapcore.Level, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(s))); err != nil {
		return level, fmt.Errorf("log.level 取值非法: %s", s)
	}
	return level, nil
}