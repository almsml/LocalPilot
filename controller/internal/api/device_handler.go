// ============================================================
// device_handler.go — 设备相关 HTTP API
// ============================================================

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/localpilot/controller/internal/registry"
)

// DeviceHandler 处理设备相关的 HTTP 请求
type DeviceHandler struct {
	registry *registry.DeviceRegistry
}

// ListDevices 返回所有设备列表
//
// GET /api/devices
// 响应：
//
//	{
//	  "devices": [
//	    {
//	      "id": "...",
//	      "hostname": "pi4",
//	      "state": "ONLINE",
//	      "cpu_percent": 0.35,
//	      ...
//	    }
//	  ]
//	}
func (h *DeviceHandler) ListDevices(c *gin.Context) {
	devices := h.registry.ListDevices()
	c.JSON(http.StatusOK, gin.H{"devices": devices})
}

// GetDevice 返回单个设备详情
//
// GET /api/devices/:id
func (h *DeviceHandler) GetDevice(c *gin.Context) {
	id := c.Param("id")
	device, err := h.registry.GetDevice(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, device)
}
