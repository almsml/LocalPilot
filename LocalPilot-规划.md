# LocalPilot — 个人设备网格计算平台

> **一句话**：把你吃灰的旧电脑、树莓派、旧手机变成你的「私人 AWS 集群」——提交任务，自动调度，并行计算。

---

## 一、项目定位

### 1.1 一句话叙事（面试开场）

> 「我有台旧 MacBook 和两个树莓派吃灰很久了，而主力机跑重任务时经常 CPU 满载。我就想：为什么不能像 AWS 那样，把我手边的设备组成一个计算集群？于是做了 LocalPilot——一个能在 5 分钟内把多台设备变成私人计算网格的系统。」

### 1.2 一句话叙事（面试官追问"有什么特别的"）

> 「它不是一个玩具 Demo。它解决了三个真问题：**异构设备的自动发现与组网**、**基于设备能力的自适应任务调度**、**节点离线时的任务自动迁移**。而且所有通信走 mTLS，任务在沙箱里跑——你从浏览器里把一个树莓派当成计算节点用，体验却像在用云服务。」

### 1.3 核心差异化

| 维度 | 面试官预期（平庸项目） | LocalPilot（你的项目） |
|------|----------------------|----------------------|
| 网络层 | 调 HTTP 库 | 自实现 mDNS 发现 + gRPC 流式通信 |
| 调度 | 没有，或者 random | 基于设备 CPU/内存/GPU/当前负载的多维调度 |
| 容错 | 不考虑 | 心跳检测 → 任务自动迁移到其他节点 |
| 安全 | 明文传输 | mTLS 加密 + 沙箱隔离执行 |
| 性能 | 不关心 | 流式日志、增量结果传输、并行分片 |

---

## 二、系统架构

### 2.1 整体架构图

```
                              ┌──────────────────────────┐
                              │     🌐 Web Dashboard      │
                              │   React + TypeScript +     │
                              │   D3.js / Canvas 可视化    │
                              │                            │
                              │  • 设备实时状态地图        │
                              │  • 任务提交面板            │
                              │  • 任务时间线 & 日志流      │
                              │  • 集群性能分析            │
                              └──────────┬───────────────┘
                                         │ HTTP/2 + WebSocket
                              ┌──────────▼───────────────┐
                              │   🧠 Controller (Go)      │
                              │                            │
                              │  ┌──────────────────────┐ │
                              │  │    REST API Server    │ │
                              │  │  (gin / chi)          │ │
                              │  └──────────────────────┘ │
                              │  ┌──────────────────────┐ │
                              │  │    Device Registry    │ │
                              │  │  • 设备发现 (mDNS)     │ │
                              │  │  • 心跳管理           │ │
                              │  │  • 能力注册           │ │
                              │  └──────────────────────┘ │
                              │  ┌──────────────────────┐ │
                              │  │    Task Scheduler     │ │
                              │  │  • 任务队列           │ │
                              │  │  • 多维匹配调度        │ │
                              │  │  • 分片策略           │ │
                              │  │  • 故障迁移           │ │
                              │  └──────────────────────┘ │
                              │  ┌──────────────────────┐ │
                              │  │    Result Aggregator  │ │
                              │  │  • 结果收集           │ │
                              │  │  • 分片合并           │ │
                              │  │  • 校验               │ │
                              │  └──────────────────────┘ │
                              │  ┌──────────────────────┐ │
                              │  │    Auth / mTLS        │ │
                              │  └──────────────────────┘ │
                              └──────────┬───────────────┘
                                         │ gRPC (protobuf)
                                         │ Service Discovery (mDNS)
                       ┌─────────────────┼─────────────────┐
                       ▼                 ▼                 ▼
              ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
              │ ⚙️ Agent (Rust)│ │ ⚙️ Agent (Rust)│ │ ⚙️ Agent (Rust)│
              │   MacBook     │ │   Mac Mini     │ │  Raspberry Pi │
              │               │ │               │ │               │
              │ • 能力上报     │ │ • 能力上报     │ │ • 能力上报     │
              │ • 任务执行     │ │ • 任务执行     │ │ • 任务执行     │
              │ • 沙箱隔离     │ │ • 沙箱隔离     │ │ • 沙箱隔离     │
              │ • 资源监控     │ │ • 资源监控     │ │ • 资源监控     │
              │ • 日志流式推送  │ │ • 日志流式推送  │ │ • 日志流式推送  │
              └──────────────┘ └──────────────┘ └──────────────┘
```

### 2.2 数据流

```
[1] 设备上线
Agent 启动 → mDNS 广播 (服务名: _localpilot._tcp)
→ Controller 发现 → mTLS 握手 → gRPC Register()
→ Controller 存入 Device Registry
→ Web Dashboard 实时更新设备列表

[2] 心跳
Agent 每 5 秒 → gRPC Heartbeat(cpu, mem, temp, running_jobs...)
→ Controller 更新设备状态
→ 超过 15 秒无心跳 → 标记为 UNHEALTHY
→ 超过 30 秒无心跳 → 标记为 OFFLINE → 触发任务迁移

[3] 任务提交
用户 Dashboard 提交任务 → REST API
→ Controller 分析任务（是否可分片？预估资源？）
→ Scheduler 匹配设备（打分：算力 × 0.4 + 空闲内存 × 0.3 + 网络延迟 × 0.2 + GPU × 0.1）
→ 分配到最优设备 → gRPC Execute(task)
→ Agent 拉取输入文件 → 沙箱执行 → 实时推送日志
→ 完成 → 上传结果 → Controller 通知用户

[4] 故障迁移
设备离线 → Controller 检测
→ 将该设备的 running task 重新入队
→ Scheduler 分配到下一个最优设备
→ 如果有 checkpoint → 从断点恢复
→ 无 checkpoint → 从头执行
```

---

## 三、技术选型

### 3.1 总览

| 组件 | 语言 | 核心理由 |
|------|------|---------|
| **Agent（工作节点）** | **Rust** | ① 零成本抽象，极致性能 ② 编译成单二进制，无运行时依赖，树莓派也能跑 ③ 内存安全，沙箱外的代码不能崩 ④ 简历上写"Rust"比"Go"更稀缺 |
| **Controller（控制中心）** | **Go** | ① 并发和网络编程是一等公民（goroutine + channel）② 快速开发，不牺牲性能 ③ 调度器天然适合 goroutine 模型 ④ 单二进制部署 |
| **Dashboard（前端）** | **TypeScript + React** | ① 生态最成熟 ② 实时数据可视化库丰富 ③ 类型安全 |
| **通信协议** | **gRPC + Protobuf** | ① 强类型 IDL，服务契约清晰 ② 内置流式传输（日志推送）③ HTTP/2 多路复用 ④ 简历加分项 |
| **服务发现** | **mDNS** | ① 零配置局域网发现 ② 标准协议，跨平台 ③ 无需中心化 DNS 服务器 |
| **数据库** | **SQLite (WAL 模式)** | ① 零运维，内嵌式 ② WAL 模式支持并发读写 ③ 足够支撑数百台设备规模 ④ 轻量 |
| **沙箱** | **Docker SDK + 备选方案** | ① Docker 最通用 ② Linux 备选：seccomp + namespace ③ macOS 备选：sandbox-exec |

### 3.2 语言混用的合理性（面试必问）

> **为什么不用一种语言全栈？为什么 Agent 用 Rust 但 Controller 用 Go？**

回答模板：

> 「这是刻意做的**异构系统设计**——在实际的分布式系统中，不同节点有不同的约束。Agent 跑在树莓派这种资源受限的设备上，我需要一个零运行时开销、内存极其节约的语言——Rust 编译成单二进制，2MB，开机自启，几乎不占内存。Controller 需要处理大量并发连接和调度逻辑，Go 的 goroutine 模型让这个天然简洁——1000 行就能写完调度器核心逻辑，换成 Rust 可能要 3000 行且异步生命周期管理非常痛苦。这是基于**约束选择工具**，而不是盲目统一。」

### 3.3 关键库选择

#### Agent (Rust)
```toml
[dependencies]
tonic = "0.12"              # gRPC 框架（最成熟的 Rust gRPC 实现）
prost = "0.13"              # Protobuf 代码生成
tokio = "1"                 # 异步运行时
mdns-sd = "0.11"            # mDNS 服务发现
bollard = "0.17"            # Docker API Rust 客户端（容器管理）
sysinfo = "0.32"            # 系统信息采集（CPU/内存/温度）
tracing = "0.1"             # 结构化日志
rustls = "0.23"             # TLS（纯 Rust 实现，不依赖 OpenSSL）
```

#### Controller (Go)
```go
require (
    google.golang.org/grpc        // gRPC Go 实现
    google.golang.org/protobuf    // Protobuf
    github.com/gin-gonic/gin      // HTTP 框架
    github.com/gorilla/websocket  // WebSocket（日志推送到前端）
    github.com/hashicorp/mdns     // mDNS 服务发现
    github.com/mattn/go-sqlite3   // SQLite 驱动
    github.com/google/uuid        // UUID 生成
)
```

#### Dashboard (React)
```json
{
  "@tanstack/react-query": "异步状态管理",
  "recharts": "设备指标图表",
  "@xyflow/react": "设备拓扑可视化",
  "react-hot-toast": "通知提示",
  "ansi-to-html": "终端日志渲染"
}
```

---

## 四、实施计划（5 个月，20 周）

### Phase 0：原型验证 — 第 1-2 周

**目标**：验证核心技术可行性，不是写产品代码

```
□ 手动在两台设备间建立 gRPC 连接（一台 Go，一台 Rust）
□ 验证 mDNS 能发现局域网内的设备
□ Rust Agent 读取 sysinfo 并上报给 Go Controller
□ 在 Controller 收到上报后，Web 页面显示设备信息
```

**产出**：一个 200 行的原型，证明 Rust↔Go gRPC 通信 + mDNS 发现的链路是通的。

**风险点**：
- mDNS 在 Windows 上的行为可能与 macOS/Linux 不同
- Rust `tonic` 和 Go gRPC 的 proto 生成路径要统一

---

### Phase 1：设备网格底座 — 第 3-6 周

**目标**：有一个真正的"集群"——多设备发现、注册、心跳、可视化

#### 第 3-4 周 | Agent 核心
```
Rust Agent 实现：
□ mDNS 广播 (service: _localpilot._tcp)
□ gRPC Register(device_info) → Controller
□ 心跳循环 (每 5 秒 gRPC Heartbeat)
□ 系统信息采集 (sysinfo: CPU %, 内存使用, 磁盘, 温度)
□ 优雅退出 (SIGTERM → Deregister)

要点：
- 心跳不要阻塞主循环，用 tokio::spawn
- DeviceInfo 包含：hostname, os, arch, cpu_cores, total_ram, gpu_info, supported_runtimes
```

#### 第 5-6 周 | Controller 设备管理 + Dashboard 初版
```
Go Controller 实现：
□ mDNS 监听 → 发现 Agent → gRPC 回连注册
□ Device Registry (in-memory + SQLite 持久化)
□ 心跳超时检测 (goroutine ticker)
□ REST API: GET /api/devices, GET /api/devices/:id
□ WebSocket: 设备上下线事件推送到 Dashboard

Dashboard 实现：
□ 设备列表卡片（在线/离线状态、CPU/内存柱状图）
□ 设备上线/下线动画通知
□ 点击设备 → 详情面板（历史负载曲线）

要点：
- Device Registry 用 sync.RWMutex 保护并发读写
- 心跳超时：15s UNHEALTHY → 30s OFFLINE
- Dashboard 用 React Query 管理实时状态
```

**产出**：3 台设备上线后，Dashboard 能实时看到 CPU 曲线跳动；断掉一台设备的 Agent，Dashboard 上 30 秒内变红。

---

### Phase 2：任务执行引擎 — 第 7-10 周

**目标**：能提交任务、调度到设备、在沙箱执行、实时看日志

#### 第 7-8 周 | 任务模型 + 基础执行
```
Proto 定义：
□ Task { id, command, args[], env{}, input_files[], 
         resource_limits{cpu_limit, mem_limit, timeout_sec} }
□ TaskStatus { QUEUED → ASSIGNED → RUNNING → COMPLETED / FAILED }
□ LogChunk { task_id, stdout/stderr, timestamp, seq_num }

Agent 执行器：
□ 接收 Task → 创建临时工作目录
□ 拉取输入文件（HTTP GET from Controller file server）
□ 执行命令 (std::process::Command + 管道捕获 stdout/stderr)
□ 流式推送日志 (gRPC server-streaming)
□ 完成后 → 上传结果文件 + 清理工作目录

Controller Job Queue：
□ 基于 channel 的内存队列
□ Job 状态机 + SQLite 持久化
□ REST API: POST /api/jobs, GET /api/jobs/:id, GET /api/jobs/:id/logs
```

#### 第 9-10 周 | 沙箱 + Dashboard 任务面板
```
沙箱（多层降级策略）：
□ 检测 Docker 可用 → 使用 bollard 创建容器执行
   - CPU/Memory limit 通过 Docker resource constraints
   - 网络隔离 (默认无网络，可选开启)
   - 只读根文件系统 + 临时可写层
□ Docker 不可用 → 进程级隔离
   - Linux: cgroups v2 + namespaces (通过 systemd-run 或直接设置)
   - macOS: sandbox-exec (App Sandbox profile)
   - 资源限制: setrlimit (RLIMIT_CPU, RLIMIT_AS)

Dashboard：
□ 任务提交表单（命令 + 资源限制 + 目标设备选择）
□ 实时终端日志（ANSI 颜色渲染）
□ 任务状态流转动效
```

**关键 Demo**：
> 在 Dashboard 输入 `ffmpeg -i input.mp4 -vf scale=1920:1080 output.mp4`
> → 任务被调度到树莓派
> → Dashboard 上看到实时的 ffmpeg 输出日志
> → 完成后自动下载结果文件

---

### Phase 3：智能调度 — 第 11-14 周

**目标**：这是项目的**技术深度核心**——多维打分调度 + 分片并行

#### 第 11-12 周 | 多维调度器
```
设备打分模型：
Score(device, task) = 
    cpu_score    * 0.35   // 空闲 CPU 比例 → 偏好负载低的设备
  + mem_score    * 0.25   // 空闲内存 → 大内存任务优先分配
  + arch_score   * 0.20   // 架构匹配 → ARM 任务不分配到 x86
  + latency      * 0.10   // 网络延迟 → 流式日志更流畅
  + gpu_score    * 0.10   // GPU 需求匹配

其中：
- cpu_score = available_cores / task.required_cores (cap at 1.0)
- 如果设备不支持 task 要求的 runtime → score = 0（直接排除）

调度策略（支持多种，可切换）：
1. BestFit: 选得分最高的设备（默认）
2. RoundRobin: 轮流分配（对比基准）
3. LeastLoaded: 选负载最低的（对比基准）

实现：
□ Scheduler 作为独立 goroutine，监听 job channel
□ 每次调度记录日志：为什么选 A 设备而不是 B 设备（用于调试）
□ 支持手动指定目标设备（覆盖自动调度）
```

#### 第 13-14 周 | 任务分片
```
可分片任务识别：
□ 用户在提交任务时标注分片策略（或自动检测）
□ 支持的分片模式：
  1. RANGE:  按数字范围分片（ffmpeg 按帧范围、图片批量处理）
  2. CHUNK:  按输入文件分片（文件列表分成 N 份）
  3. PARAM:  按参数组合分片（网格搜索、超参调优）

分片执行流程：
User 提交: "用 3 台设备并行转码视频"
→ Controller 分析视频 → 按关键帧切分成 3 段
→ 3 个 Task 分别发到 3 个 Agent
→ Agent 各自转码一段 → 上传中间结果
→ Controller 用 ffmpeg concat 合并最终视频

Dashboard 展示：
□ 分片任务 → 显示子任务进度条矩阵
□ 总进度: [████████░░] 78% (3/4 完成)
□ 每个子任务的设备名 + 实时日志可展开查看
```

**关键 Demo（面试必展示）**：
> 一个 4K 视频转码 → 3 台设备并行处理
> → 单设备耗时 12 分钟 → LocalPilot 耗时 4.5 分钟
> → **3 倍加速，线性可扩展**

---

### Phase 4：容错与韧性 — 第 15-17 周

**目标**：让系统不是"玩具"——节点挂了不影响任务

#### 第 15-16 周 | 故障检测 + 任务迁移
```
故障检测：
□ 心跳超时 → UNHEALTHY (15s) → OFFLINE (30s)
□ UNHEALTHY 阶段：给设备一次"自救"机会（重连）
□ OFFLINE 阶段：确认死亡，触发任务迁移

任务迁移：
□ 设备 OFFLINE → Controller 查找该设备上所有 RUNNING 任务
□ 任务重新入队（保留原 task_id + 递增 attempt_number）
□ Scheduler 分配到下一台最优设备
□ 通知 Dashboard：显示迁移动画 (任务从一个设备飞到另一个)
□ 如果有 checkpoint → 从 checkpoint 恢复

Checkpoint 机制（可选，第 17 周做）：
□ Agent 定期 (每 30 秒) 上传任务中间状态到 Controller
□ 中间状态包括：stdout/stderr 增量 + 工作目录 diff
□ 恢复时：新 Agent 下载 checkpoint → 重建工作目录 → 重试
□ 对于不可恢复的任务（如网络请求） → 标记为 FAILED_IMMUTABLE
```

#### 第 17 周 | 网络韧性
```
网络不稳定处理：
□ Agent 检测心跳失败 → 指数退避重连 (1s, 2s, 4s, 8s, max 30s)
□ Controller 侧：UNHEALTHY → 维持一段宽限期再标记 OFFLINE
□ gRPC keepalive：防止防火墙断开空闲连接

文件传输韧性：
□ 断点续传（HTTP Range 请求）
□ 文件校验（SHA256）
□ 失败重试 (最多 3 次)
```

**关键 Demo**：
> 任务在树莓派上跑着 → 拔掉树莓派电源
> → Dashboard 上设备变红 → 15 秒后任务自动迁移到 Mac Mini
> → Mac Mini 从 checkpoint 恢复 → 继续执行 → 最终完成
> → **整个过程中用户无需任何操作**

---

### Phase 5：打磨与面试准备 — 第 18-20 周

#### 第 18 周 | Dashboard 打磨
```
□ 设备拓扑图（React Flow）：节点之间的连线表示当前任务流转
□ 实时任务甘特图：横轴时间，纵轴设备，显示任务调度历史
□ 集群总览仪表盘：总 CPU 利用率、总内存、活跃任务数、累计完成数
□ 暗色模式（面试 Demo 好看）
```

#### 第 19 周 | Demo 视频准备
```
准备 3 个录屏 Demo：
1. 【30 秒快闪】：3 台设备上线 → 任务提交 → 并行执行 → 完成
   （演示核心流程，节奏快，视觉效果好）

2. 【技术深度】：展示调度打分过程 + 设备宕机自动迁移
   （演示你的架构设计能力）

3. 【性能对比】：单设备 vs 3 设备并行，展示加速比曲线
   （演示你的性能意识）
```

#### 第 20 周 | 文档 + README
```
□ README.md：项目概述、架构图、快速开始
□ ARCHITECTURE.md：架构设计文档（面试官可能会提前看）
□ DEMO.md：Demo 场景脚本 + 性能数据
□ 代码注释补齐
□ 录制一个 5 分钟的 Loom/哔哩哔哩视频
```

---

## 五、关键 Demo 场景

### Demo 1：并行视频转码（主力展示）
```
准备: 一段 4K/10 分钟的测试视频, 3 台设备（MacBook + Mac Mini + 树莓派）

操作:
1. Dashboard 拖入视频文件 → 选择「并行转码 1080p」
2. 系统自动分片 → 3 个 Worker 并行处理
3. 实时看到 3 条日志流同时推进
4. 2 分钟后完成，自动合并 → 一键下载

台词:
> 「一台 MacBook 单独转这个视频要 5 分钟，用 LocalPilot 调度三台设备， 
> 加上分片开销，2 分钟搞定。更多的设备 = 更快的速度，线性扩展。」
```

### Demo 2：故障迁移（展示架构深度）
```
准备: 树莓派上运行一个长时间任务（如大文件压缩）

操作:
1. 任务在树莓派上跑着
2. 拔掉树莓派电源
3. Dashboard 设备卡片变红 → 30 秒后任务自动迁移到 Mac Mini
4. Mac Mini 继续执行 → 最终完成

台词:
> 「分布式系统的核心不是'不出问题'，而是'出问题后怎么恢复'。
> LocalPilot 做到了 30 秒内检测故障、自动重调度、零人工干预。」
```

### Demo 3：异构能力匹配（展示调度器）
```
准备: 一个需要 GPU 的 AI 推理任务, 一个有 GPU 的 MacBook, 两个无 GPU 的树莓派

操作:
1. 提交 AI 推理任务
2. 调度器自动排除了两个树莓派（无 GPU）
3. 任务自动匹配到有 GPU 的 MacBook

台词:
> 「调度器不是随机选设备——它理解每台设备的能力，确保 GPU 任务不会
> 被发到树莓派上。这是一个能理解异构硬件的调度系统。」
```

---

## 六、面试话术库

### 6.1 为什么选 Rust 写 Agent、Go 写 Controller？

> 「在实际的分布式系统中，不同节点面临的约束是不同的。Agent 要跑在树莓派上——Rust 编译成单二进制，2MB，内存占用极低，没有运行时开销。Controller 要处理大量并发调度——Go 的 goroutine 让每个心跳、每个任务调度都是一个轻量协程。这是基于**约束选择工具**，不是为了炫技。」

### 6.2 这个项目最大的技术挑战是什么？

> 「挑战在**系统中立**——你不是在做一个层面的优化。网络层要处理设备发现和断连，调度层要做多维匹配，执行层要做沙箱隔离，UI 层要做实时可视化。最难的其实是**让这些层在一起稳定工作**。比如设备宕机时，要同时更新注册表 → 取消调度 → 重分配任务 → 通知前端，这四步要保证一致性。」

### 6.3 如果给你更多时间，你会怎么改进？

> 「三个方向：① **NAT 穿透**，让不在同一局域网的设备也能组网，比如用 Tailscale/Headscale 做 overlay network。② **更细粒度的沙箱**——用 WebAssembly (Wasmtime) 替代 Docker 来做毫秒级启动的微隔离，这个在技术上非常前沿。③ **分布式文件系统**——目前文件传输是中心化的（Controller 中转），后续可以做成 P2P chunk 分发，类似 BitTorrent。」

### 6.4 你的调度器比 Kubernetes 差在哪？好在哪？

> 「K8s 面对的是数据中心里的同构节点，调度考虑 CPU/内存/亲和性。LocalPilot 面对的是**极度异构**的个人设备——一台 MacBook 有 GPU，树莓派是 ARM，Windows 不支持 Docker——调度器必须理解这些异构能力。K8s 的调度器是通用解，LocalPilot 的调度器是**面向个人异构环境的专用解**，更轻量，也更容易理解。」

### 6.5 系统能承受多少设备？

> 「理论上限受 SQLite 和 mDNS 协议约束——mDNS 在同一局域网建议不超过 100 台设备（这是协议本身的设计范围）。实际测试目标：10 台设备稳定运行。这不是一个云服务级别的系统，而是**个人/小团队的网格计算工具**，定位明确。」

---

## 七、技术风险与对策

| 风险 | 概率 | 影响 | 对策 |
|------|------|------|------|
| mDNS 在 Windows 上行为不一致 | 中 | 中 | Phase 0 第一时间在 3 个平台上验证，如果不稳定则增加手动 IP 输入备选方案 |
| Rust `tonic` TLS 配置复杂 | 中 | 中 | Phase 1 先用明文通信，Phase 4 再加 mTLS；`tonic` 的 TLS 文档在 2025 年已有大幅改善 |
| gRPC 在弱网下表现差 | 低 | 高 | gRPC 本身有 HTTP/2 keepalive；额外增加应用层心跳超时检测 |
| 树莓派上 Docker 性能差 | 中 | 低 | 提供无 Docker 的进程级执行备选方案（Linux cgroups + namespaces） |
| 任务分片的视频合并有音画不同步 | 中 | 低 | 用 ffmpeg 的关键帧精确切分，Phase 3 花时间调参 |

---

## 八、项目文件结构

```
localpilot/
├── README.md                       # 项目概述 + 架构图 + 快速开始
├── ARCHITECTURE.md                 # 架构设计文档
├── Makefile                        # 统一构建命令
├── proto/                          # Protobuf 定义（单一事实来源）
│   └── localpilot/
│       └── v1/
│           ├── agent.proto         # Agent ↔ Controller 通信
│           ├── task.proto          # 任务模型
│           └── discovery.proto     # 服务发现
│
├── agent/                          # Rust Agent
│   ├── Cargo.toml
│   └── src/
│       ├── main.rs                 # 入口：启动 mDNS + gRPC server
│       ├── discovery.rs            # mDNS 广播 & 服务注册
│       ├── transport.rs            # gRPC client (连接 Controller)
│       ├── executor.rs             # 任务执行器 (命令执行 + 日志捕获)
│       ├── sandbox.rs              # 沙箱抽象层 (Docker / 进程隔离)
│       ├── monitor.rs              # 系统资源监控 (sysinfo)
│       ├── heartbeat.rs            # 心跳循环
│       └── config.rs               # 配置管理
│
├── controller/                     # Go Controller
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   └── controller/
│   │       └── main.go             # 入口
│   ├── internal/
│   │   ├── api/
│   │   │   ├── router.go           # HTTP 路由 (gin)
│   │   │   ├── device_handler.go   # 设备相关 API
│   │   │   ├── job_handler.go      # 任务相关 API
│   │   │   └── ws_handler.go       # WebSocket (实时推送)
│   │   ├── registry/
│   │   │   ├── device.go           # Device Registry (内存 + SQLite)
│   │   │   └── health.go           # 心跳检测 & 健康管理
│   │   ├── scheduler/
│   │   │   ├── scheduler.go        # 调度器核心
│   │   │   ├── scorer.go           # 多维打分引擎
│   │   │   └── splitter.go         # 任务分片策略
│   │   ├── job/
│   │   │   ├── manager.go          # Job 生命周期管理
│   │   │   ├── queue.go            # Job Queue
│   │   │   └── store.go            # SQLite 持久化
│   │   ├── transport/
│   │   │   └── grpc.go             # gRPC client (连接 Agent)
│   │   └── discovery/
│   │       └── mdns.go             # mDNS 监听
│   └── pkg/
│       └── proto/                  # 生成的 protobuf Go 代码
│
├── dashboard/                      # React 前端
│   ├── package.json
│   ├── vite.config.ts
│   ├── src/
│   │   ├── App.tsx
│   │   ├── api/
│   │   │   └── client.ts           # API 客户端 (React Query hooks)
│   │   ├── components/
│   │   │   ├── DeviceGrid.tsx      # 设备网格/列表
│   │   │   ├── DeviceCard.tsx      # 设备卡片（状态、指标）
│   │   │   ├── DeviceDetail.tsx    # 设备详情面板
│   │   │   ├── JobSubmitter.tsx    # 任务提交表单
│   │   │   ├── JobTimeline.tsx     # 任务时间线
│   │   │   ├── TerminalLog.tsx     # 实时终端日志
│   │   │   ├── ClusterTopology.tsx # 集群拓扑图 (React Flow)
│   │   │   └── MetricsDashboard.tsx# 集群指标总览
│   │   ├── hooks/
│   │   │   ├── useWebSocket.ts     # WebSocket hook
│   │   │   └── useDevices.ts       # 设备状态 hook
│   │   └── types/
│   │       └── index.ts            # TypeScript 类型定义
│
├── scripts/
│   ├── setup-dev.sh                # 开发环境搭建
│   └── demo-scenario.sh            # Demo 场景自动化脚本
│
└── docs/
    ├── SETUP.md                    # 部署指南
    └── DEMOS.md                    # Demo 场景 + 脚本
```

---

## 九、每周时间投入建议

```
前提：假设你每周能投入 15-20 小时（课余时间 + 周末）

Phase 0 (周 1-2):   ═══════█░░░░░░░░░░░░░░░  原型验证, 风险探索
Phase 1 (周 3-6):   ═══════════███░░░░░░░░░░  设备底座, 60% Rust + 30% Go + 10% React
Phase 2 (周 7-10):  ════════════════███░░░░░  任务执行, 30% Rust + 40% Go + 30% React
Phase 3 (周 11-14): ════════════════════███░  智能调度, 10% Rust + 60% Go + 30% React
Phase 4 (周 15-17): ═══════════════════════██  容错韧性, 30% Rust + 50% Go + 20% React
Phase 5 (周 18-20): ════════════════════════█  打磨面试, 文档 + Demo + README

Demo 产出节奏：
  Phase 1 结束 → "多设备在线监控" (可录 30 秒视频)
  Phase 2 结束 → "远程执行命令 + 实时日志" (可录 1 分钟)
  Phase 3 结束 → "并行视频转码" (核心 Demo)
  Phase 4 结束 → "拔电源自动迁移" (深度 Demo)
  Phase 5 结束 → 完整 Demo 视频 + 项目文档
```

---

## 十、面试官视角预判

### 面试官可能会问的"刁钻"问题

| 问题 | 你的回答要点 |
|------|------------|
| 「这和 Ansible/ SaltStack 有什么区别？」 | 「Ansible 是配置管理，面向的是服务器运维；LocalPilot 是**计算调度**，面向的是把闲置算力变成计算集群。定位完全不同。」 |
| 「为什么不直接用 Kubernetes？」 | 「K8s 假设你的节点是同构的云端 VM，LocalPilot 假设节点是极度异构的个人设备；而且 K8s 的最小部署复杂度远超个人场景需要的。」 |
| 「调度器能处理资源超卖吗？」 | 「目前不支持。这是一个显式设计决策——个人设备集群的资源是真实的物理资源，超卖反而会让用户对'加速'的预期落空。后续可以加入弹性超卖。」 |
| 「gRPC 比 HTTP 好在哪？」 | 「强类型契约是分布式系统的生命线。Protobuf 生成的代码保证 Agent 和 Controller 的通信不会因为字段名拼写错误而运行时挂掉。流式传输也比 WebSocket 更适合结构化的日志推送。」 |
| 「怎么保证任务隔离的安全性？」 | 「多层降级：有 Docker → 容器隔离；没有 Docker → Linux namespace + seccomp 进程隔离，限制网络和文件系统访问。这不是安全产品级别的隔离，但足够防止误操作。」 |

---

## 十一、如果你做完了所有 Phase，还有这些「进阶方向」

> 以下只写在规划里，不一定要实现。但如果你被问到「后续想做什么」，这些都是极好的回答。

1. **WASM 微隔离**：用 WebAssembly (Wasmtime) 替代 Docker，毫秒级冷启动，比容器轻 100 倍的沙箱
2. **NAT 穿透组网**：用 WireGuard 或 Tailscale 做 overlay network，让不在同一 WiFi 的设备也能组队
3. **移动端 Worker**：让旧 Android 手机（通过 Termux）也变成计算节点
4. **分布式文件系统**：P2P chunk 分发替代中心化文件传输
5. **GPU 内存池**：把多台设备的 GPU 显存虚拟化成一个统一的内存池，跑大模型推理
6. **自动扩缩容**：设备空闲时自动加入集群 → 用户使用时自动退出，无需手动管理

---

> **最后一条建议**：把这个文档的 README 写好，架构图画漂亮，Demo 视频录清晰——这三样东西比你写 10000 行代码更有面试说服力。面试官花 30 秒看你的项目，前 5 秒看架构图，接下来 15 秒看 Demo，最后 10 秒决定要不要追问。
