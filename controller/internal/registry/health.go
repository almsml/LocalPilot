// ============================================================
// health.go — 心跳超时检测
//
// 为什么心跳检测在 Controller 侧而不是 Agent 侧？
//   Agent 只负责发心跳（"我还在"）。判断设备是否离线是 Controller 的职责——
//   因为只有 Controller 知道全局状态，需要在设备离线时触发任务迁移。
//
// 为什么分两层（UNHEALTHY → OFFLINE）而不是一步到位？
//   网络波动很常见（WiFi 信号不稳定、交换机重启）。
//   UNHEALTHY 给设备 15 秒的"自救"时间——如果能在这期间重连成功，
//   就不触发任务迁移。只有确认死亡（30 秒）后才迁移。
//   这避免了"网络卡了一下 → 任务迁移到另一台 → 原设备又恢复了"的混乱。
// ============================================================

package registry

import (
	"log"
	"time"
)

const (
	// HeartbeatUnhealthyThreshold 心跳超时阈值 → UNHEALTHY（秒）
	HeartbeatUnhealthyThreshold = 15 * time.Second

	// HeartbeatOfflineThreshold 心跳超时阈值 → OFFLINE（秒）
	HeartbeatOfflineThreshold = 30 * time.Second
)

// StartHealthChecker 启动心跳超时检测循环
//
// 作为独立 goroutine 运行，每隔 checkInterval 检查所有设备的 LastHeartbeat。
// 超时的设备状态会根据阈值从 ONLINE → UNHEALTHY → OFFLINE 递进。
//
// # 参数
//   - checkInterval: 检查间隔，建议 1 秒
func (r *DeviceRegistry) StartHealthChecker(checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		r.checkHealth()
	}
}

// checkHealth 遍历所有设备，检查心跳是否超时
func (r *DeviceRegistry) checkHealth() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	for id, device := range r.devices {
		elapsed := now.Sub(device.LastHeartbeat)

		oldState := device.State

		switch {
		case elapsed > HeartbeatOfflineThreshold:
			// 30 秒无心跳 → 确认离线
			if device.State != StateOffline {
				device.State = StateOffline
				log.Printf("[健康检查] 设备 OFFLINE: %s (%s), 最后心跳: %v ago",
					device.Hostname, id, elapsed.Round(time.Second))
				// Phase 4: 触发任务迁移
				// migrateTasksFromDevice(device.ID)
			}
		case elapsed > HeartbeatUnhealthyThreshold:
			// 15 秒无心跳 → 标记为可疑
			if device.State == StateOnline {
				device.State = StateUnhealthy
				log.Printf("[健康检查] 设备 UNHEALTHY: %s (%s), 最后心跳: %v ago",
					device.Hostname, id, elapsed.Round(time.Second))
			}
		default:
			// 心跳正常（但之前可能是不健康状态，现在恢复了）
			if device.State != StateOnline {
				log.Printf("[健康检查] 设备恢复: %s (%s) → ONLINE", device.Hostname, id)
				device.State = StateOnline
			}
		}

		// 状态变化时通知 Dashboard（通过 WebSocket）
		if oldState != device.State {
			r.notifyStateChange(device, oldState)
		}
	}
}

// notifyStateChange 设备状态变化时通知前端
//
// Phase 2 实现：通过 WebSocket 向所有连接的 Dashboard 推送状态变化事件。
// 当前 Phase 0：只打日志。
func (r *DeviceRegistry) notifyStateChange(device *Device, oldState DeviceState) {
	// TODO(Phase 2): 通过 WebSocket hub 广播状态变化到 Dashboard
	log.Printf("[状态变化] %s: %s → %s", device.Hostname, oldState, device.State)
}
