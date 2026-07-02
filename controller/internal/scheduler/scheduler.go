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

// SelectDevice 为任务选择最优执行设备
//
// 内部调用多维打分引擎（scorer.go），对每台在线设备
// 按 CPU/内存/架构/延迟/GPU 五个维度综合评分，
// 返回得分最高的设备。
//
// 如果没有在线设备，返回 nil。
//
// Phase 3: 评分不依赖任务细节（任务尚未声明资源需求）。
// Phase 4 会加入任务级别的 GPU/架构需求匹配。
func (s *Scheduler) SelectDevice() *registry.Device {
	return SelectBest(s.registry)
}
