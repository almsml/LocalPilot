// ============================================================
// job_handler.go — 任务相关 HTTP API
//
// Phase 2: 完整的任务提交和查询接口。
// ============================================================

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/localpilot/controller/internal/job"
)

// JobHandler 处理任务相关的 HTTP 请求
type JobHandler struct {
	manager *job.Manager
}

// NewJobHandler 创建任务处理器
func NewJobHandler(manager *job.Manager) *JobHandler {
	return &JobHandler{manager: manager}
}

// ---- 请求/响应类型 ----

// SubmitJobRequest 任务提交请求体
type SubmitJobRequest struct {
	Name    string            `json:"name"`
	Command string            `json:"command" binding:"required"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// SubmitJob 提交新任务
//
// POST /api/jobs
// 请求体：
//
//	{
//	  "name": "测试任务",
//	  "command": "echo",
//	  "args": ["hello", "world"],
//	  "env": {"FOO": "bar"}
//	}
func (h *JobHandler) SubmitJob(c *gin.Context) {
	var req SubmitJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误: " + err.Error()})
		return
	}

	// 创建任务
	j, err := h.manager.SubmitJob(req.Name, req.Command, req.Args, req.Env)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建任务失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, j)
}

// GetJob 获取任务详情（含日志）
//
// GET /api/jobs/:id
func (h *JobHandler) GetJob(c *gin.Context) {
	id := c.Param("id")
	j, err := h.manager.GetJob(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务未找到"})
		return
	}

	c.JSON(http.StatusOK, j)
}

// ListJobs 列出所有任务
//
// GET /api/jobs
func (h *JobHandler) ListJobs(c *gin.Context) {
	jobs, err := h.manager.ListJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务列表失败"})
		return
	}
	if jobs == nil {
		jobs = []*job.Job{}
	}
	c.JSON(http.StatusOK, gin.H{"jobs": jobs})
}
