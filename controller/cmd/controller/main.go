// ============================================================
// main.go — LocalPilot Controller 入口
//
// Controller 是系统的中央协调器。职责（按启动顺序）：
//   1. 初始化 SQLite 数据库（WAL 模式）
//   2. 初始化设备注册表（内存 + SQLite）
//   3. 启动 mDNS 监听器 → 发现局域网内的 Agent
//   4. 启动 gRPC 服务器 → Agent 通过它注册/心跳
//   5. 启动 HTTP 服务器（gin）→ Dashboard 通过它获取数据
//   6. 启动调度器 → 将任务分配到最优设备
//   7. 启动心跳超时检测 → 发现离线设备并迁移任务
//
// 为什么用 Go 写 Controller？
//   Controller 需要处理大量并发连接：每台 Agent 的 gRPC 心跳、
//   Dashboard 的 WebSocket 推送、任务调度。Go 的 goroutine 让
//   每个连接、每个心跳、每个调度决策都是一个轻量协程。
// ============================================================

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/localpilot/controller/internal/api"
	"github.com/localpilot/controller/internal/discovery"
	"github.com/localpilot/controller/internal/registry"
	"github.com/localpilot/controller/internal/scheduler"
	"github.com/localpilot/controller/internal/transport"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("LocalPilot Controller 启动中...")

	// ---- 1. 初始化数据库 ----
	// 为什么用 SQLite？
	//   零运维——不需要单独部署数据库服务。
	//   WAL 模式支持一写多读并发，足够支撑数百台设备。
	dbPath := getEnv("LOCALPILOT_DB_PATH", "./localpilot.db")
	store, err := registry.NewSQLiteStore(dbPath)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer store.Close()
	log.Printf("SQLite 数据库已连接: %s (WAL 模式)", dbPath)

	// ---- 2. 初始化设备注册表 ----
	// DeviceRegistry 维护所有在线设备的内存缓存 + SQLite 持久化。
	// 用 sync.RWMutex 保护并发读写——读操作（API 查询）远多于写操作（心跳更新）。
	devRegistry := registry.NewDeviceRegistry(store)
	log.Println("设备注册表已初始化")

	// ---- 2.5. 从 SQLite 恢复设备列表 ----
	// Controller 重启后，之前注册的设备信息仍在 SQLite 中。
	// 恢复到内存缓存，初始状态设为 OFFLINE——Agent 重连后会自动上线。
	if loadedDevices, err := store.LoadDevices(); err != nil {
		log.Printf("从数据库恢复设备列表失败: %v", err)
	} else if len(loadedDevices) > 0 {
		devRegistry.RestoreDevices(loadedDevices)
		log.Printf("从数据库恢复了 %d 台设备（状态：OFFLINE，等待 Agent 重连）", len(loadedDevices))
	}

	// ---- 3. 启动心跳超时检测 ----
	// 独立 goroutine，每秒检查一次所有设备的心跳时间戳。
	// 15 秒无心跳 → UNHEALTHY，30 秒 → OFFLINE。
	go devRegistry.StartHealthChecker(1*time.Second)

	// ---- 4. 初始化调度器 ----
	// 调度器从任务队列中取任务，根据设备能力打分，分配到最优设备。
	sch := scheduler.NewScheduler(devRegistry)

	// ---- 5. 启动 gRPC 服务器 ----
	// Controller 是 gRPC server——Agent 调用 Controller 的 Register/Heartbeat/Deregister。
	grpcPort := getEnv("LOCALPILOT_GRPC_PORT", "50051")
	grpcServer := transport.NewGRPCServer(devRegistry, sch)
	go func() {
		log.Printf("gRPC 服务启动在 0.0.0.0:%s", grpcPort)
		if err := grpcServer.Serve(":" + grpcPort); err != nil {
			log.Fatalf("gRPC 服务启动失败: %v", err)
		}
	}()

	// ---- 6. 启动 HTTP 服务器（gin） ----
	// 为 Dashboard 提供 REST API + WebSocket。
	httpPort := getEnv("LOCALPILOT_HTTP_PORT", "8080")
	router := api.NewRouter(devRegistry, sch)
	go func() {
		log.Printf("HTTP API 服务启动在 0.0.0.0:%s", httpPort)
		if err := http.ListenAndServe(":"+httpPort, router); err != nil {
			log.Fatalf("HTTP 服务启动失败: %v", err)
		}
	}()

	// ---- 7. 启动 mDNS 监听（可选） ----
	// mDNS 监听让 Controller 自动发现局域网内广播的 Agent。
	// 为什么默认关闭？
	//   Windows 上 mDNS 多播可能因防火墙或网络配置失败。
	//   设置为 LOCALPILOT_ENABLE_MDNS=true 启用。
	if getEnv("LOCALPILOT_ENABLE_MDNS", "false") == "true" {
		go func() {
			if err := discovery.ListenForAgents(devRegistry); err != nil {
				log.Printf("mDNS 监听启动失败: %v", err)
			}
		}()
		log.Println("mDNS 监听已启动 (_localpilot._tcp)")
	} else {
		log.Println("mDNS 监听已禁用（设置 LOCALPILOT_ENABLE_MDNS=true 启用）")
	}

	// ---- 等待退出信号 ----
	log.Println("✅ LocalPilot Controller 已就绪")
	log.Printf("   gRPC: localhost:%s", grpcPort)
	log.Printf("   HTTP: http://localhost:%s", httpPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("收到信号 %v，正在优雅关闭...", sig)

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	grpcServer.Stop(ctx)
	log.Println("Controller 已关闭")
}

// getEnv 读取环境变量，不存在时返回默认值
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
