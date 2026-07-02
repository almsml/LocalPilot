// ============================================================
// heartbeat.go — 心跳循环 + 重连信号（Phase 4）
//
// Phase 4: 心跳连续失败超过阈值时，通过 reconnectCh 通知
// main goroutine 触发与 Controller 的完整重连流程。
// ============================================================

package heartbeat

import (
	"context"
	"log/slog"
	"time"

	"github.com/localpilot/agent/internal/monitor"
	"github.com/localpilot/agent/internal/transport"
	pb "github.com/localpilot/proto/localpilot/v1"
)

// Run 启动心跳循环
//
// Phase 4 新增 reconnectCh: 需要重连时发送信号。
// main goroutine 收到信号后执行重连→重注册→重启心跳。
func Run(
	ctx context.Context,
	client *transport.DeviceServiceClient,
	deviceID string,
	heartbeatInterval uint32,
	reconnectCh chan<- struct{},
) {
	slog.Info("心跳循环启动", "device_id", deviceID, "interval_sec", heartbeatInterval)

	consecutiveFailures := 0
	const reconnectThreshold = 6 // 30 秒后触发重连

	ticker := time.NewTicker(time.Duration(heartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("心跳循环退出", "device_id", deviceID)
			return

		case <-ticker.C:
			err := sendHeartbeat(ctx, client, deviceID)
			if err != nil {
				consecutiveFailures++
				if consecutiveFailures == 3 {
					slog.Warn("心跳连续失败 3 次", "device_id", deviceID, "error", err)
				} else if consecutiveFailures >= reconnectThreshold {
					slog.Error("心跳连续失败，触发重连", "device_id", deviceID, "failures", consecutiveFailures)
					select {
					case reconnectCh <- struct{}{}:
					default:
					}
					return
				}
			} else {
				if consecutiveFailures > 0 {
					slog.Info("心跳恢复", "device_id", deviceID, "previous_failures", consecutiveFailures)
				}
				consecutiveFailures = 0
			}
		}
	}
}

// sendHeartbeat 发送一次心跳
func sendHeartbeat(ctx context.Context, client *transport.DeviceServiceClient, deviceID string) error {
	usage := monitor.CollectResourceUsage()
	req := &pb.HeartbeatRequest{
		DeviceId:      deviceID,
		ResourceUsage: usage,
	}
	sendCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := client.Heartbeat(sendCtx, req)
	if err != nil {
		return err
	}
	slog.Debug("心跳发送成功", "device_id", deviceID)
	return nil
}
