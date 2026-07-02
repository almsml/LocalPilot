// ============================================================
// router.go — HTTP API 路由（gin）
//
// Dashboard 通过这个 API 获取设备列表、提交任务、查看日志。
// ============================================================

package api

import (
	"github.com/gin-gonic/gin"
	"github.com/localpilot/controller/internal/job"
	"github.com/localpilot/controller/internal/registry"
	"github.com/localpilot/controller/internal/scheduler"
)

// NewRouter 创建 gin 路由并注册所有 API 端点
func NewRouter(devReg *registry.DeviceRegistry, sch *scheduler.Scheduler, jobMgr *job.Manager) *gin.Engine {
	r := gin.Default()
	r.Use(corsMiddleware())

	// 设备相关 API
	deviceHandler := &DeviceHandler{registry: devReg}
	api := r.Group("/api")
	{
		api.GET("/devices", deviceHandler.ListDevices)
		api.GET("/devices/:id", deviceHandler.GetDevice)
	}

	// 任务相关 API
	jobHandler := NewJobHandler(jobMgr)
	{
		api.POST("/jobs", jobHandler.SubmitJob)
		api.GET("/jobs", jobHandler.ListJobs)
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
