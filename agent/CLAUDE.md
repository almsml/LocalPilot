# agent/ — Go Agent（工作节点守护进程）

## 一句话

Agent 是跑在每台计算设备上的后台进程。开机自启，自动注册到 Controller，等待任务下发，在沙箱中执行任务，实时推送日志。

## 为什么改用 Go？

原本规划使用 Rust，2026-07-01 决定统一到 Go：

- **单一语言栈**：后端只有 Go，降低认知负担和维护成本
- **共享 proto 代码**：Agent 和 Controller 通过 `replace` 指令共享同一份 proto 生成的 Go 代码（`pkg/proto/`）
- **统一交叉编译**：`GOOS=linux GOARCH=arm64 go build` 一条命令出 ARM 二进制
- **Go 编译出 ~16MB 二进制**，在旧设备上完全可接受
- **goroutine 模型**天然适合心跳循环和 gRPC 服务并发

## 模块地图

```
cmd/agent/main.go            — 入口：初始化日志→加载配置→连接 Controller→注册→心跳循环→gRPC 服务→等待信号→优雅退出
internal/
├── config/config.go         — 配置管理（环境变量覆盖，Phase 0 无配置文件）
├── monitor/monitor.go       — 系统监控（CPU 核心数/OS/架构/总内存/Docker 检测/GPU 检测）
├── monitor/monitor_windows.go — Windows 内存信息（GlobalMemoryStatusEx API）
├── monitor/monitor_mem_other.go — 非 Windows 平台内存 fallback
├── transport/
│   ├── client.go            — gRPC DeviceService 客户端（Register/Heartbeat/Deregister）
│   └── server.go            — gRPC TaskExecutionService 服务端（Execute/CancelTask）
├── heartbeat/heartbeat.go   — 心跳循环（goroutine + time.Ticker，每 5 秒）
├── discovery/mdns.go        — mDNS 广播（Phase 1 实现）
├── executor/executor.go     — 任务执行器（Phase 2 实现）
└── sandbox/sandbox.go       — 沙箱隔离（Phase 2 实现）
```

## 关键设计决策

1. **为什么用 goroutine + Ticker 而不是 time.Sleep？**
   Ticker 的间隔固定——不受心跳请求耗时影响。如果心跳请求需要 200ms，sleep 模式间隔变成 5s+200ms=5.2s，累积后会偏移。

2. **为什么 gRPC 服务在 goroutine 中启动？**
   与 Rust 版本不同，Go 的 gRPC server.Serve() 本身就阻塞。我们在 goroutine 中启动它，main goroutine 继续等待退出信号。

3. **为什么心跳用独立的 context 控制生命周期？**
   心跳 goroutine 需要在 Agent 退出时优雅停止。通过 `context.WithCancel` 创建的心跳 ctx，main 在收到 SIGTERM 时调用 `cancel()`，心跳收到 `ctx.Done()` 后退出。

4. **为什么 monitor 用标准库而不是 gopsutil？**
   Phase 0 的目标是验证 gRPC 链路。标准库 + golang.org/x/sys 提供基本的系统信息，避免额外依赖。Phase 1 会迁移到 gopsutil 以获得更精确的实时指标。

## 编译与运行

```bash
# 编译（需要 Go 1.24+）
cd agent && go build ./cmd/agent/

# 运行（连接本地 Controller）
LOCALPILOT_CONTROLLER_HOST=127.0.0.1 go run ./cmd/agent/

# 运行（连接远程 Controller）
LOCALPILOT_CONTROLLER_HOST=192.168.1.100 go run ./cmd/agent/

# 交叉编译到 ARM64（旧笔记本/手机）
GOOS=linux GOARCH=arm64 go build -o agent-arm64 ./cmd/agent/

# 交叉编译到 macOS
GOOS=darwin GOARCH=amd64 go build -o agent-darwin-amd64 ./cmd/agent/
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `LOCALPILOT_HOSTNAME` | 系统 hostname | 设备名 |
| `LOCALPILOT_CONTROLLER_HOST` | 127.0.0.1 | Controller 地址 |
| `LOCALPILOT_CONTROLLER_PORT` | 50051 | Controller gRPC 端口 |
| `LOCALPILOT_AGENT_PORT` | 50052 | Agent gRPC 监听端口 |

## 当前状态：Phase 0

- ✅ Go 项目骨架（go.mod + 9 个源文件）
- ✅ 配置管理（环境变量 + 跨平台 hostname）
- ✅ 系统监控（CPU 核心数/OS/架构/总内存/GPU 检测/Docker 检测）
- ✅ gRPC DeviceService 客户端（Connect/Register/Heartbeat/Deregister）
- ✅ gRPC TaskExecutionService 服务端（Execute/CancelTask）
- ✅ 心跳循环（goroutine + Ticker）
- ⏳ mDNS 广播（Phase 1）
- ⏳ 任务执行器（Phase 2）
- ⏳ 沙箱隔离（Phase 2）
