// ============================================================
// health.go — 心跳超时检测 + 设备离线回调
//
// Phase 4: 设备变为 OFFLINE 时触发任务迁移回调，
// job manager 收到回调后将设备上的任务重新入队。
// ============================================================

package registry

import (
	"log"
	"log/slog"
	"time"
)

const (
	HeartbeatUnhealthyThreshold = 15 * time.Second // → UNHEALTHY
	HeartbeatOfflineThreshold   = 30 * time.Second // → OFFLINE
)

// DeviceOfflineCallback 设备被标记为 OFFLINE 时的回调
type DeviceOfflineCallback func(deviceID string)

// onDeviceOffline 由 job manager 注册
var onDeviceOffline DeviceOfflineCallback

// SetDeviceOfflineCallback 注册设备离线回调
func SetDeviceOfflineCallback(cb DeviceOfflineCallback) {
	onDeviceOffline = cb
}

// StartHealthChecker 启动心跳超时检测
func (r *DeviceRegistry) StartHealthChecker(checkInterval time.Duration) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for range ticker.C {
		r.checkHealth()
	}
}

// checkHealth 检查所有设备心跳，推进状态机
func (r *DeviceRegistry) checkHealth() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for id, device := range r.devices {
		elapsed := now.Sub(device.LastHeartbeat)
		oldState := device.State

		switch {
		case elapsed > HeartbeatOfflineThreshold:
			if device.State != StateOffline {
				device.State = StateOffline
				log.Printf("[健康检查] 设备 OFFLINE: %s (%s)", device.Hostname, id)
				r.triggerOfflineCallback(id)
			}
		case elapsed > HeartbeatUnhealthyThreshold:
			if device.State == StateOnline {
				device.State = StateUnhealthy
				log.Printf("[健康检查] 设备 UNHEALTHY: %s (%s)", device.Hostname, id)
			}
		default:
			if device.State != StateOnline {
				log.Printf("[健康检查] 设备恢复: %s (%s) → ONLINE", device.Hostname, id)
				device.State = StateOnline
			}
		}

		if oldState != device.State {
			r.notifyStateChange(device, oldState)
		}
	}
}

func (r *DeviceRegistry) triggerOfflineCallback(deviceID string) {
	if onDeviceOffline != nil {
		go func() {
			slog.Info("触发任务迁移", "device_id", deviceID)
			onDeviceOffline(deviceID)
		}()
	}
}

func (r *DeviceRegistry) notifyStateChange(device *Device, oldState DeviceState) {
	log.Printf("[状态变化] %s: %s → %s", device.Hostname, oldState, device.State)
}
