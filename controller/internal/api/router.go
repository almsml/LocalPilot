// ============================================================
// router.go — HTTP API 路由（gin）
//
// Dashboard 通过这个 API 获取设备列表、提交任务、查看日志。
// 为什么用 gin？
//   轻量、高性能、中间件生态成熟。
//   路由分组 + 中间件让 API 结构清晰——/api/devices 和 /api/jobs 各自独立。
// ============================================================

package api

import (
	"github.com/gin-gonic/gin"
	"github.com/localpilot/controller/internal/registry"
	"github.com/localpilot/controller/internal/scheduler"
)

// NewRouter 创建 gin 路由并注册所有 API 端点
//
// API 结构：
//   GET  /api/devices       — 设备列表
//   GET  /api/devices/:id   — 设备详情
//   POST /api/jobs           — 提交任务
//   GET  /api/jobs/:id       — 任务详情
//   GET  /api/jobs/:id/logs  — 任务日志
//   GET  /ws                 — WebSocket 连接（实时状态推送）
func NewRouter(devReg *registry.DeviceRegistry, sch *scheduler.Scheduler) *gin.Engine {
	r := gin.Default()

	// CORS 中间件——允许 Dashboard 开发服务器（Vite:5173）跨域访问
	r.Use(corsMiddleware())

	// 设备相关 API
	deviceHandler := &DeviceHandler{registry: devReg}
	api := r.Group("/api")
	{
		api.GET("/devices", deviceHandler.ListDevices)
		api.GET("/devices/:id", deviceHandler.GetDevice)
	}

	// 任务相关 API
	jobHandler := &JobHandler{scheduler: sch}
	{
		api.POST("/jobs", jobHandler.SubmitJob)
		api.GET("/jobs/:id", jobHandler.GetJob)
	}

	// WebSocket 实时推送
	wsHandler := &WSHandler{registry: devReg}
	r.GET("/ws", wsHandler.HandleWebSocket)

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return r
}

// corsMiddleware 允许跨域请求
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
