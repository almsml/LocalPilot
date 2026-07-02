// ============================================================
// scheduler.go — 任务调度器核心
//
// Phase 3 实现完整的多维打分调度。
// Phase 0: 返回占位类型。
// ============================================================

package scheduler

import "github.com/localpilot/controller/internal/registry"

// Scheduler 任务调度器
type Scheduler struct {
	registry *registry.DeviceRegistry
}

// NewScheduler 创建调度器
func NewScheduler(reg *registry.DeviceRegistry) *Scheduler {
	return &Scheduler{registry: reg}
}
