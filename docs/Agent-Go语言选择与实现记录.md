# Agent Go 语言选择与实现记录

> 日期：2026-07-01 ~ 2026-07-02  
> 最终状态：编译通过，gRPC 链路就绪，实时系统监控正常工作

---

## 一、为什么 Agent 选择 Go？

### 真实原因

**Rust 我没学会。**

最初规划 Agent 用 Rust、Controller 用 Go。Controller 很快写完了，但 Rust Agent 进展很慢：

- Rust 的所有权/借用/生命周期概念需要大量时间消化
- `tokio` 异步运行时的 `Pin`/`Unpin`/`'static` 生命周期标注对新手来说心智负担太大
- `tonic`（Rust gRPC 框架）和 `sysinfo`（系统监控库）的 API 文档不如 Go 生态直观
- 调试 Rust 编译错误的时间远超写业务逻辑的时间

继续死磕 Rust 会严重影响项目进度。所以决定：**Agent 也用 Go，和 Controller 统一**。

### 技术层面的合理性

虽然最初原因很朴素——"学不会 Rust"——但这个选择在技术上也是合理的：

1. **Go 我已经会了**：Controller 就是用 Go 写的，写 Agent 不需要重新学语言
2. **共享 proto 代码**：Agent 和 Controller 引用同一份 proto 生成的 Go 代码（`pkg/proto/`），不用维护两套生成逻辑
3. **交叉编译同样简单**：`GOOS=linux GOARCH=arm64 go build` 一条命令出 ARM 二进制给 ARM 设备用
4. **二进制体积可接受**：Go 编译出 ~16MB，在旧设备上完全没问题
5. **goroutine 天然适合这个场景**：每 5 秒一次心跳、长时间运行的任务执行、gRPC 流式通信——Go 的并发模型天生匹配

**面试时如果被问"为什么不用 Rust？"**：

> 我最初确实尝试了 Rust，但 Rust 的学习曲线在项目时间压力下不划算。我评估后觉得 Go 完全满足需求——Agent 本质是一个网络守护进程，瓶颈在 I/O 不在 CPU，Go 的性能绰绰有余。而且用同一门语言写两个组件，开发和维护效率都更高。这是一个务实的选择。

---

## 二、做了什么

### 架构

```
Dashboard (React/TS)
    │ HTTP + WebSocket
    ▼
Controller (Go)  ←──────── gRPC ─────────→  Agent (Go)
  端口 50051         Register/Heartbeat     端口 50052
  端口 8080 (HTTP)   Execute/CancelTask     运行在每台计算设备上
```

三个组件、两种语言（Go + TypeScript），后端全部 Go。

### Agent 代码结构

```
agent/
├── cmd/agent/main.go               — 入口：启动→注册→心跳→gRPC服务→等待退出
├── internal/
│   ├── config/config.go             — 环境变量配置
│   ├── monitor/monitor.go           — 系统监控（gopsutil 主路径 + 平台降级）
│   ├── monitor/monitor_windows.go   — Windows 内存 API 实现
│   ├── monitor/monitor_mem_other.go — 非 Windows 平台 fallback
│   ├── transport/client.go          — gRPC 客户端（调 Controller 的 DeviceService）
│   ├── transport/server.go          — gRPC 服务端（实现 TaskExecutionService）
│   ├── heartbeat/heartbeat.go       — 心跳循环
│   ├── discovery/mdns.go            — mDNS 占位（Phase 1）
│   ├── executor/executor.go         — 任务执行占位（Phase 2）
│   └── sandbox/sandbox.go           — 沙箱占位（Phase 2）
├── go.mod
└── CLAUDE.md
```

### 共享 proto 代码

```
pkg/proto/                           — 从 Controller 中抽出来，双方共用
├── go.mod
└── localpilot/v1/
    ├── agent.pb.go / agent_grpc.pb.go   — 设备注册/心跳/任务执行
    ├── task.pb.go                       — 任务模型/状态机/分片
    └── discovery.pb.go                  — 服务发现

Agent 和 Controller 都通过 replace 指令引用：
  replace github.com/localpilot/proto => ../pkg/proto
```

---

## 三、每个模块的设计思路

### 3.1 `cmd/agent/main.go` — 启动流程

```
1. 初始化日志（slog，Go 标准库的结构化日志）
2. config.Load()              ← 读环境变量
3. monitor.CollectDeviceInfo() ← 采集本机 CPU/内存/GPU/Docker 信息
4. transport.Connect()        ← 连 Controller 的 gRPC（Phase 0 明文连接）
5. client.Register()          ← 注册设备，拿到 device_id
6. go heartbeat.Run()         ← 独立 goroutine 跑心跳循环
7. transport.StartTaskServer() ← 启动 gRPC 任务执行服务
8. 等待 SIGINT/SIGTERM
9. 优雅退出：停心跳 → 注销设备 → 关 gRPC
```

每一步失败都会 `os.Exit(1)` 并打印错误——**Fail Fast**，不试图挽救。连不上 Controller 的 Agent 没有存在的意义，让 systemd 重试。

### 3.2 `internal/config/config.go` — 配置

全部走环境变量，没有配置文件。原因：Agent 部署在不同设备上，文件路径不统一，环境变量是 Docker/12-Factor App 的标准做法。

```
LOCALPILOT_CONTROLLER_HOST → 默认 127.0.0.1
LOCALPILOT_CONTROLLER_PORT → 默认 50051
LOCALPILOT_AGENT_PORT      → 默认 50052
LOCALPILOT_HOSTNAME        → 默认 os.Hostname()
```

### 3.3 `internal/monitor/monitor.go` — 系统监控

采集两种数据：

| 类型 | 时机 | 内容 | 谁用 |
|------|------|------|------|
| 静态信息 | 注册时 | OS/架构/CPU 核心数/总内存/GPU/可用运行时 | 调度器判断设备能跑什么 |
| 实时指标 | 心跳时 | CPU 使用率/已用内存/温度/运行中任务数 | 调度器判断设备忙不忙 |

**实现策略：gopsutil 主路径 + 平台 API 降级**

```
cpu.Percent(100ms)        → 成功：返回 CPU 使用率
                          → 失败：返回 0

mem.VirtualMemory()       → 成功：返回 总内存/已用内存
                          → 失败：走平台 API
                                  ├─ Windows: GlobalMemoryStatusEx
                                  ├─ Linux:   /proc/meminfo
                                  └─ macOS:   sysctl + vm_stat

sensors.SensorsTemperatures() → 成功：返回 CPU 温度
                               → 失败：返回 -1
```

这叫**优雅降级（Graceful Degradation）**——监控库出 bug 不影响 Agent 启动，只是指标精度下降。

**GPU 检测**：调度器打分模型有 10% 权重给 GPU。AI 推理任务只能分配到有 GPU 的设备。

```
macOS:  system_profiler SPDisplaysDataType → Chipset Model
Linux:  检查 /proc/driver/nvidia → nvidia-smi 获取型号
Windows: Phase 0 简化
```

**Docker 检测**：沙箱可以跑在 Docker 容器里（最强隔离），也可以退到进程级隔离。

```
Linux/macOS: /var/run/docker.sock 存在？
Windows:     \\.\pipe\docker_engine 存在？
```

### 3.4 `internal/transport/client.go` — gRPC 客户端

Agent 作为 client 调 Controller 的 DeviceService：

```
Connect()     → 建连接（Phase 0 明文，Phase 4 加 TLS）
Register()    → 上报设备信息，拿到 device_id
Heartbeat()   → 每 5 秒上报资源使用
Deregister()  → 退出时从集群移除
Close()       → 释放连接
```

封装一层的好处：业务逻辑只调 `client.Register(info)`，不关心底层 gRPC 怎么连的。将来换 TLS、加重连逻辑，只改这一个文件。

### 3.5 `internal/transport/server.go` — gRPC 服务端

Agent 同时也是 server，Controller 会调过来下发任务：

```
Execute(req, stream) → 收到任务，通过 stream.Send() 实时推送日志
CancelTask(ctx, req) → 取消正在跑的任务
```

**为什么 Execute 用 streaming 而不是一次性返回？**

- 不用 streaming：Agent 执行 10 分钟 → 一次性返回全部日志 → 用户干等
- 用 streaming：Agent 每有新输出就推 → Dashboard 实时滚动 → 能中途取消

### 3.6 `internal/heartbeat/heartbeat.go` — 心跳

```go
func Run(ctx, client, deviceID, interval) {
    ticker := time.NewTicker(5 * time.Second)
    for {
        select {
        case <-ctx.Done():      // main 调了 cancel，准备退出
            return
        case <-ticker.C:        // 每 5 秒
            usage := monitor.CollectResourceUsage()  // 采指标
            client.Heartbeat(usage)                  // 上报
        }
    }
}
```

**为什么用 `time.Ticker` 而不是 `time.Sleep`？**

`Sleep` 的问题：sleep(5s) → 发心跳耗时 200ms → sleep(5s)，实际间隔变成 5.2s，长期累积偏移。
`Ticker`：固定每 5s 触发一次，不受心跳本身耗时影响。

**为什么用 context 控制退出？**

Agent 退出时的顺序很重要：
1. 停心跳 → Controller 不再收到心跳
2. 发 Deregister → 主动告知"我下线了"
3. 关 gRPC 服务

反过来（先关服务再注销）的话，Controller 要等 30 秒超时才发现设备离线。

### 3.7 `internal/monitor/monitor_windows.go` — Windows 内存

直接调 kernel32.dll 的 `GlobalMemoryStatusEx` 拿物理内存信息。

**为什么用 Ex 版本？**

老版 `GlobalMemoryStatus` 用 32 位字段存内存值，超过 4GB 就溢出返回错误数据。`Ex` 版用 64 位字段，支持现代大内存设备。这是 Windows API 设计中的经典教训。

---

## 四、遇到的问题

### 4.1 go mod tidy 在无网络时失败

`go mod tidy` 会解析所有传递依赖包括测试依赖。gRPC 的测试依赖了 opentelemetry、gonum 等十几个包，离线环境拉不下来。

**解决**：不用 `go mod tidy`，直接用 `go build`（只解析编译依赖）。日常维护 go.mod 靠手动编辑。

### 4.2 gopsutil v4 的 API 变化

gopsutil v3 → v4 时，温度传感器 API 从 `host` 包移到了 `sensors` 包，`TemperatureStat` 的 `Label` 字段改名 `SensorKey`。对着文档改就好，五分钟的事。

### 4.3 proto 生成代码中的旧路径

生成的 `.pb.go` 文件在二进制描述符里嵌了旧模块路径。不影响编译——Go 编译器只看 `package v1` 声明，不读那些 bytes。将来重新跑一下 `protoc` 就行。

---

## 五、依赖清单

| 包 | 版本 | 用途 |
|----|------|------|
| `google.golang.org/grpc` | v1.82.0 | gRPC 通信框架 |
| `google.golang.org/protobuf` | v1.36.11 | Protobuf 序列化 |
| `github.com/shirou/gopsutil/v4` | v4.26.6 | 跨平台系统监控（CPU/内存/温度） |
| `golang.org/x/sys` | v0.44.0 | Windows 系统调用（降级路径） |

---

## 六、Phase 0 当前状态

| 模块 | 状态 |
|------|------|
| 配置管理（环境变量） | ✅ |
| gRPC DeviceService 客户端（注册/心跳/注销） | ✅ |
| gRPC TaskExecutionService 服务端（执行/取消） | ✅ |
| 心跳循环（每 5 秒） | ✅ |
| 系统监控（CPU/内存/温度/GPU/Docker） | ✅ |
| mDNS 广播 | ⏳ Phase 1 |
| 任务执行器 | ⏳ Phase 2 |
| 沙箱隔离 | ⏳ Phase 2 |

## 七、数据流

```
Dashboard ──HTTP──▶ Controller ──gRPC──▶ Agent
  │                    │                    │
  │  GET /api/devices  │  Register()        │  采集系统指标
  │  {devices: [...]}  │  Heartbeat()       │  执行任务
  │                    │  Execute()         │  推送日志
  ▼                    ▼                    ▼
 浏览器              设备注册表            计算设备
                     SQLite
```

两步通信：
1. **Agent → Controller**：Agent 主动调用 Register/Heartbeat/Deregister
2. **Controller → Agent**：Controller 调用 Execute 下发任务，Agent 通过 streaming 把日志推回来

---

## 八、运行

```bash
# 终端 1：Controller
cd controller && go run ./cmd/controller/

# 终端 2：Agent
cd agent && LOCALPILOT_CONTROLLER_HOST=127.0.0.1 go run ./cmd/agent/

# 终端 3：Dashboard
cd dashboard && npm run dev
# http://localhost:5173
```

交叉编译给 ARM 设备：

```bash
cd agent
GOOS=linux GOARCH=arm64 go build -o agent-arm64 ./cmd/agent/
```
