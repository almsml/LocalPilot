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

	// 连续失败计数器——用于检测 Controller 不可达
	// 为什么需要计数器而不是每次都打 ERROR？
	//   偶尔的网络抖动（1-2 次失败）是正常的，打 ERROR 会误导排查。
	//   连续失败才说明真的有问题——Controller 挂了或网络断了。
	consecutiveFailures := 0

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
			err := sendHeartbeat(ctx, client, deviceID)
			if err != nil {
				consecutiveFailures++

				// 分级告警——不同严重程度用不同日志级别
				// 为什么 3 次才打 WARN？
				//   1-2 次失败可能只是网络抖动（WiFi 信道切换、交换机瞬时拥塞）。
				//   3 次（15 秒）说明 Controller 可能真的不可达了。
				// 为什么 6 次打 ERROR？
				//   6 次（30 秒）已经超过 Controller 的 OFFLINE 阈值——
				//   Controller 已经把这台设备标记为离线了。
				if consecutiveFailures == 3 {
					slog.Warn("心跳连续失败 3 次，Controller 可能不可达",
						"device_id", deviceID,
						"error", err,
					)
				} else if consecutiveFailures == 6 {
					slog.Error("心跳连续失败 6 次（30 秒），设备可能已被标记为离线",
						"device_id", deviceID,
						"error", err,
					)
				} else {
					slog.Debug("心跳发送失败",
						"device_id", deviceID,
						"consecutive_failures", consecutiveFailures,
						"error", err,
					)
				}
			} else {
				// 一次成功就重置计数器——说明链路恢复了
				if consecutiveFailures > 0 {
					slog.Info("心跳恢复", "device_id", deviceID,
						"previous_failures", consecutiveFailures)
				}
				consecutiveFailures = 0
			}
		}
	}
}

// sendHeartbeat 发送一次心跳，返回错误供调用方判断
//
// 为什么改为返回 error 而不是在内部处理？
//   调用方（Run 循环）需要计数连续失败次数，
//   所以把错误传递出去让循环统一管理计数器。
func sendHeartbeat(
	ctx context.Context,
	client *transport.DeviceServiceClient,
	deviceID string,
) error {
	// 采集当前资源使用情况
	usage := monitor.CollectResourceUsage()

	req := &pb.HeartbeatRequest{
		DeviceId:      deviceID,
		ResourceUsage: usage,
	}

	// 设置独立的超时时间，防止单次心跳请求 hang 住整个循环
	// 为什么是 3 秒？
	//   LAN 环境下 gRPC 调用通常 < 50ms。3 秒已经给了足够余量
	//   （慢设备、WiFi 波动），但不会让心跳循环阻塞太久。
	sendCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := client.Heartbeat(sendCtx, req)
	if err != nil {
		return err
	}

	slog.Debug("心跳发送成功", "device_id", deviceID)
	return nil
}
