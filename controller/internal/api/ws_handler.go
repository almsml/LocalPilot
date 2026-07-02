// ============================================================
// ws_handler.go — WebSocket 实时推送
//
// Phase 2 实现。Phase 0 返回 501。
// ============================================================

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/localpilot/controller/internal/registry"
)

// WSHandler 处理 WebSocket 连接
type WSHandler struct {
	registry *registry.DeviceRegistry
}

// HandleWebSocket 升级 HTTP 连接为 WebSocket
//
// GET /ws
// Phase 2 实现：推送设备上下线事件、实时日志到 Dashboard。
func (h *WSHandler) HandleWebSocket(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Phase 2 实现"})
}
