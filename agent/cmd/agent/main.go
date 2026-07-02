// ============================================================
// main.go — LocalPilot Agent 入口（Go 版本）
//
// Agent 的职责（按启动顺序）：
//   1. 加载配置（Controller 地址、设备名等）
//   2. 初始化日志系统（slog）
//   3. 采集本机系统信息
//   4. 连接 Controller 的 gRPC 服务
//   5. 向 Controller 注册（gRPC Register）
//   6. 启动心跳循环（goroutine）→ 每 5 秒上报状态
//   7. 启动 gRPC 服务器 → 等待 Controller 下发任务
//   8. 等待退出信号 → 注销 → 清理
//
// 为什么 main.go 是流程编排而不是逻辑实现？
//   把具体实现放在 internal/ 子包中，main.go 只负责
//   按正确的顺序启动和串联各模块。
//
// 与 Rust 版本 (agent/src/main.rs) 对应。
// ============================================================

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/localpilot/agent/internal/config"
	"github.com/localpilot/agent/internal/heartbeat"
	"github.com/localpilot/agent/internal/monitor"
	"github.com/localpilot/agent/internal/transport"
)

// ============================================================
// main — Agent 启动入口
// ============================================================

func main() {
	// ---- 0. 初始化日志 ----
	// 为什么用 slog 而不是 log 包？
	//   slog 是 Go 1.21+ 的结构化日志标准库，支持
	//   键值对日志、按级别过滤、JSON 输出。
	//   部署到树莓派后可以设 LOG_LEVEL=info 只看关键信息，
	//   调试时设 LOG_LEVEL=debug 看详细信息。
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	slog.Info("LocalPilot Agent 启动中...")

	// ---- 1. 加载配置 ----
	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置加载失败", "error", err)
		os.Exit(1)
	}
	slog.Info("配置加载完成", "detail", cfg.String())

	// ---- 2. 采集本机系统信息 ----
	deviceInfo := monitor.CollectDeviceInfo(cfg)

	// ---- 3. 连接 Controller ----
	slog.Info("正在连接 Controller...")
	ctx := context.Background()
	client, err := transport.Connect(ctx, cfg.ControllerHost, cfg.ControllerPort)
	if err != nil {
		slog.Error("连接 Controller 失败", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	// ---- 4. 向 Controller 注册 ----
	resp, err := client.Register(ctx, deviceInfo)
	if err != nil {
		slog.Error("设备注册失败", "error", err)
		os.Exit(1)
	}

	slog.Info("注册成功",
		"device_id", resp.DeviceId,
		"heartbeat_interval_sec", resp.HeartbeatIntervalSec,
	)

	// ---- 5. 启动心跳循环 ----
	// 用 context 控制心跳生命周期。
	// cancel 函数在退出时调用，通知心跳 goroutine 停止。
	heartbeatCtx, cancelHeartbeat := context.WithCancel(context.Background())
	defer cancelHeartbeat()

	deviceID := resp.DeviceId
	heartbeatInterval := resp.HeartbeatIntervalSec

	go heartbeat.Run(heartbeatCtx, client, deviceID, heartbeatInterval)
	slog.Info("心跳循环已启动", "interval_sec", heartbeatInterval)

	// ---- 6. 启动 gRPC 任务执行服务 ----
	taskServer, err := transport.StartTaskServer(cfg.AgentPort)
	if err != nil {
		slog.Error("gRPC 任务服务启动失败", "error", err)
		os.Exit(1)
	}
	defer taskServer.Stop()

	// ---- 7. 等待退出信号 ----
	slog.Info("LocalPilot Agent 已就绪",
		"device_id", deviceID,
		"gprc_port", cfg.AgentPort,
	)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	slog.Info("收到退出信号，正在优雅关闭...", "signal", sig.String())

	// 1. 停止心跳
	cancelHeartbeat()

	// 2. 向 Controller 注销
	deregisterCtx := context.Background()
	if err := client.Deregister(deregisterCtx, deviceID, "user_shutdown"); err != nil {
		slog.Error("注销失败", "error", err)
	}

	// 3. 停止 gRPC 服务
	taskServer.Stop()

	slog.Info("Agent 已关闭")
}
