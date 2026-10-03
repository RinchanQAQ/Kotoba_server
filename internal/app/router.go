package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kotoba/internal/bootstrap"
	"kotoba/internal/middleware"
	"kotoba/internal/shared/errcode"
	"kotoba/internal/shared/logging"
	"kotoba/internal/shared/response"
)

// APIPrefix 业务接口的统一版本前缀。
const APIPrefix = "/v1"

// NewRouter 组装 gin 引擎：全局中间件、兜底路由、探针路由与业务路由分组。
func NewRouter(application *bootstrap.App) *gin.Engine {
	if application.Config.App.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// 方法不匹配时返回 405 而非 404，交给 NoMethod 统一处理。
	r.HandleMethodNotAllowed = true

	// 中间件顺序即执行顺序（外 → 内），不可随意调换：
	//   RequestID      生成/透传链路 ID
	//   RequestLogger  把带 request_id 的 logger 注入 context
	//   AccessLog      记录访问日志（最外层，能观测到 Recovery 写回的 500）
	//   Recovery       panic 兜底
	//   ErrorHandler   收敛 handler 用 c.Error 上报的错误
	//   CORS           跨域
	r.Use(
		middleware.RequestID(),
		middleware.RequestLogger(application.Logger),
		middleware.AccessLog(application.Logger),
		middleware.Recovery(application.Logger),
		middleware.ErrorHandler(),
		middleware.CORS(application.Config.App.CORS.AllowOrigins),
	)

	// 兜底路由同样输出统一响应体，避免出现 gin 默认的纯文本 404。
	r.NoRoute(func(c *gin.Context) { response.AbortFail(c, errcode.ErrNotFound) })
	r.NoMethod(func(c *gin.Context) { response.AbortFail(c, errcode.ErrMethodNotAllowed) })

	registerProbes(r, application)
	registerRoutes(r, application)

	return r
}

// registerRoutes 注册 /v1 下的业务路由。
//
// 业务模块在 internal/modules/<name> 内提供
// RegisterRoutes(v1 *gin.RouterGroup, app *bootstrap.App)，在此挂载即可。
// 下面两个路由是基建自带的示例，用于验证响应格式、参数校验与日志链路。
func registerRoutes(r *gin.Engine, application *bootstrap.App) {
	v1 := r.Group(APIPrefix)

	v1.GET("/ping", func(c *gin.Context) {
		response.OK(c, gin.H{"pong": true, "env": application.Config.App.Env})
	})

	v1.POST("/echo", echo)
}

type echoRequest struct {
	Message string `json:"message" binding:"required,min=1,max=200"`
}

// echo 演示三件事：统一响应体、参数校验错误的中文翻译、带 request_id 的业务日志。
func echo(c *gin.Context) {
	var req echoRequest
	if !response.BindJSON(c, &req) {
		return
	}

	logging.From(c.Request.Context()).Info("收到 echo 请求", zap.String("message", req.Message))

	response.OK(c, gin.H{"message": req.Message, "request_id": response.RequestID(c)})
}

// registerProbes 注册存活与就绪探针。
// 探针面向运维系统，返回扁平结构以便直接对接监控，不套用业务响应体。
func registerProbes(r *gin.Engine, application *bootstrap.App) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		checks := gin.H{}
		ready := true

		if sqlDB, err := application.MySQL.DB(); err != nil {
			checks["mysql"] = "error"
			ready = false
		} else if err := sqlDB.PingContext(ctx); err != nil {
			checks["mysql"] = err.Error()
			ready = false
		} else {
			checks["mysql"] = "ok"
		}

		if err := application.Redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = err.Error()
			ready = false
		} else {
			checks["redis"] = "ok"
		}

		if !ready {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "checks": checks})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "checks": checks})
	})
}
