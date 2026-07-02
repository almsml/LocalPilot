// ============================================================
// heartbeat.go — 心跳循环
//
// 为什么心跳不能用简单的 loop + sleep？
//   goroutine 让心跳独立运行，不影响 gRPC 服务器接收任务。
//   如果心跳阻塞了主循环，Agent 将无法接收 Controller 下发的任务。
//
// 为什么心跳间隔是 5 秒？
//   这是一个权衡：
//   - 太短（1s）：浪费带宽，Controller 处理负担大
//   - 太长（30s）：设备离线后要等很久才发现
//   - 5s 是 K8s kubelet 等系统验证过的平衡值
//
// 与 Rust 版本 (agent/src/heartbeat.rs) 对应。
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

// ============================================================
// Run — 启动心跳循环
//
// 在独立的 goroutine 中运行，每 heartbeatInterval 秒发送一次心跳。
// 通过 context 控制生命周期——ctx 取消时退出循环。
//
// 与 Rust 版本 run() 对应。
// ============================================================

// Run 启动心跳循环
//
// 参数：
//   - ctx: 用于优雅退出，ctx.Done() 时停止心跳
//   - client: Controller 的 DeviceService gRPC 客户端
//   - deviceID: Controller 分配的设备 ID
//   - heartbeatInterval: 心跳间隔（秒）
func Run(
	ctx context.Context,
	client *transport.DeviceServiceClient,
	deviceID string,
	heartbeatInterval uint32,
) {
	slog.Info("心跳循环启动", "device_id", deviceID, "interval_sec", heartbeatInterval)

	// time.NewTicker 创建周期性定时器
	// 为什么用 Ticker 而不是 time.Sleep？
	//   Ticker 的间隔是固定的——不受心跳请求耗时影响。
	//   如果心跳请求本身需要 200ms，sleep 模式下间隔变成
	//   5s + 200ms = 5.2s，累积后会偏移。
	ticker := time.NewTicker(time.Duration(heartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("心跳循环收到退出信号", "device_id", deviceID)
			return

		case <-ticker.C:
			sendHeartbeat(ctx, client, deviceID)
		}
	}
}

// sendHeartbeat 发送一次心跳
//
// 与 Rust 版本 send_heartbeat() 对应。
func sendHeartbeat(
	ctx context.Context,
	client *transport.DeviceServiceClient,
	deviceID string,
) {
	// 采集当前资源使用情况
	usage := monitor.CollectResourceUsage()

	req := &pb.HeartbeatRequest{
		DeviceId:      deviceID,
		ResourceUsage: usage,
	}

	// 设置独立的超时时间，防止心跳请求 hang 住
	sendCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Heartbeat(sendCtx, req)
	if err != nil {
		slog.Error("心跳发送失败",
			"device_id", deviceID,
			"error", err,
		)
		return
	}

	slog.Debug("心跳发送成功", "device_id", deviceID)
}
