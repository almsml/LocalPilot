// ============================================================
// job_handler.go — 任务相关 HTTP API
//
// Phase 2 实现完整逻辑。Phase 0 返回空占位。
// ============================================================

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/localpilot/controller/internal/scheduler"
)

// JobHandler 处理任务相关的 HTTP 请求
type JobHandler struct {
	scheduler *scheduler.Scheduler
}

// SubmitJob 提交新任务
//
// POST /api/jobs
// Phase 2 实现
func (h *JobHandler) SubmitJob(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Phase 2 实现"})
}

// GetJob 获取任务详情
//
// GET /api/jobs/:id
// Phase 2 实现
func (h *JobHandler) GetJob(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Phase 2 实现"})
}
