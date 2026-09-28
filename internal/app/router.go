package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"kotoba/internal/bootstrap"
	"kotoba/internal/middleware"
)

// APIPrefix 业务接口的统一版本前缀。
const APIPrefix = "/v1"

// NewRouter 组装 gin 引擎：全局中间件、探针路由与业务路由分组。
func NewRouter(application *bootstrap.App) *gin.Engine {
	if application.Config.App.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.AccessLog(application.Logger),
		middleware.Recovery(application.Logger),
		middleware.CORS(application.Config.App.CORS.AllowOrigins),
	)

	registerProbes(r, application)

	// 业务模块在此注册，模块自行提供 RegisterRoutes(v1 *gin.RouterGroup, app *bootstrap.App)。
	v1 := r.Group(APIPrefix)
	_ = v1

	return r
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
