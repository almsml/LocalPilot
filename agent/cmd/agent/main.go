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
//   7. 启动 mDNS 广播 → 局域网自动发现
//   8. 启动 gRPC 服务器 → 等待 Controller 下发任务
//   9. 等待退出信号 → 注销 → 清理
//
// 为什么 main.go 是流程编排而不是逻辑实现？
//   把具体实现放在 internal/ 子包中，main.go 只负责
//   按正确的顺序启动和串联各模块。
// ============================================================

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/mdns"

	"github.com/localpilot/agent/internal/config"
	"github.com/localpilot/agent/internal/discovery"
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
	//   部署到旧设备后可以设 LOG_LEVEL=info 只看关键信息，
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

	// ---- 6. 启动 mDNS 广播 ----
	// 为什么 mDNS 在注册之后启动？
	//   注册成功说明 Controller 可达、网络正常。
	//   此时广播才有意义——Controller 已经在监听了。
	// 为什么 mDNS 失败不阻塞启动？
	//   mDNS 是辅助发现机制，不是核心功能。
	//   Agent 仍可通过 LOCALPILOT_CONTROLLER_HOST 直接连接。
	var mdnsServer *mdns.Server
	mdnsServer, err = discovery.StartMDNSBroadcast(cfg)
	if err != nil {
		slog.Warn("mDNS 广播启动失败，Agent 仍可通过直接 IP 连接", "error", err)
		mdnsServer = nil
	}

	// ---- 7. 启动 gRPC 任务执行服务 ----
	taskServer, err := transport.StartTaskServer(cfg.AgentPort)
	if err != nil {
		slog.Error("gRPC 任务服务启动失败", "error", err)
		os.Exit(1)
	}
	defer taskServer.Stop()

	// ---- 8. 等待退出信号 ----
	slog.Info("LocalPilot Agent 已就绪",
		"device_id", deviceID,
		"gprc_port", cfg.AgentPort,
	)

	// 信号通道——缓冲大小为 2，以便在极端情况下不丢失第二个信号
	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	slog.Info("收到退出信号，正在优雅关闭...", "signal", sig.String())

	// ---- 优雅关闭 ----
	// 为什么用 goroutine + select 而不是顺序执行？
	//   顺序执行的问题是：如果某一步 hang 住了（例如 Deregister
	//   因为 Controller 不可达而阻塞），后续步骤永远不会执行。
	//   goroutine + 超时 select 确保即使某个步骤卡住，
	//   也能在超时后强制退出。

	// totalShutdownTimeout 是整个优雅关闭的最大允许时间。
	// 为什么是 10 秒？
	//   Deregister 占 5 秒 + GracefulStop 占 3 秒 + 缓冲 2 秒。
	//   LAN 环境下通常 1 秒内就能完成。
	const totalShutdownTimeout = 10 * time.Second

	shutdownDone := make(chan struct{})

	go func() {
		// 第一步：停止心跳
		cancelHeartbeat()

		// 第二步：停止 mDNS 广播（如果已启动）
		if mdnsServer != nil {
			slog.Info("正在停止 mDNS 广播...")
			if err := mdnsServer.Shutdown(); err != nil {
				slog.Warn("mDNS 广播停止失败", "error", err)
			}
		}

		// 第三步：向 Controller 注销
		// 为什么 Deregister 需要独立的超时？
		//   如果 Controller 已经挂了，Deregister 会一直等待直到
		//   gRPC 连接超时（可能 2 分钟）。用 5 秒超时避免卡死。
		deregCtx, deregCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer deregCancel()

		slog.Info("正在向 Controller 注销...")
		if err := client.Deregister(deregCtx, deviceID, "user_shutdown"); err != nil {
			slog.Warn("注销失败（将在 Controller 心跳检测中过期）", "error", err)
		}

		// 第四步：停止 gRPC 任务服务
		// GracefulStop 会等待所有活跃的 gRPC 流完成，
		// 然后关闭服务器。Phase 1 中 Execute 是 stub，
		// 所以 GracefulStop 几乎立即返回。
		taskServer.Stop()

		close(shutdownDone)
	}()

	// 等待关闭完成 OR 超时 OR 第二个信号
	select {
	case <-shutdownDone:
		slog.Info("Agent 已优雅关闭")

	case <-time.After(totalShutdownTimeout):
		slog.Warn("优雅关闭超时，强制退出",
			"timeout_sec", totalShutdownTimeout/time.Second)
		os.Exit(1)

	case sig := <-quit:
		// 收到第二个 SIGTERM/SIGINT——用户等不及了
		slog.Warn("收到第二个信号，立即强制退出",
			"signal", sig.String())
		os.Exit(1)
	}
}
