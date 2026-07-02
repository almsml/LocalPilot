# CLAUDE.md

本文件为 Claude Code（claude.ai/code）在此仓库中工作时提供指导。

## 项目状态

本项目目前处于 **Phase 0 — 原型验证**阶段。

## 开发进度

### 2026-07-01 — Phase 0 骨架搭建 + gRPC 链路验证 ✅

**完成事项：**
- Proto 定义（agent.proto / task.proto / discovery.proto）
- Go Agent 项目（9 个源文件，编译通过）—— 原计划 Rust，已于同日迁移到 Go
- Go Controller 项目骨架（12 个文件，编译通过）
- React Dashboard 项目骨架（10 个文件，编译通过）
- protoc + protoc-gen-go + protoc-gen-go-grpc 工具链安装
- Controller gRPC DeviceService 完整实现（Register / Heartbeat / Deregister）
- Agent gRPC DeviceService 客户端 + TaskExecutionService 服务端实现
- Agent ↔ Controller gRPC 互通验证通过
- 心跳循环正常工作（每 5 秒）
- HTTP API 返回设备数据（GET /api/devices）
- Dashboard 设备卡片 UI 就绪

**当前可运行：**
```bash
# 终端 1：启动 Controller
cd controller && go run ./cmd/controller/

# 终端 2：启动 Agent
cd agent && LOCALPILOT_CONTROLLER_HOST=127.0.0.1 go run ./cmd/agent/

# 终端 3：启动 Dashboard
cd dashboard && npm run dev
# 浏览器打开 http://localhost:5173
```

**编译命令：**
```bash
# Go Agent
cd agent && go build ./cmd/agent/

# Go Controller
cd controller && go build ./cmd/controller/

# React Dashboard
cd dashboard && npm install && npm run dev
```

**下一步（Phase 1 — 设备网格底座）：**
- Agent mDNS 广播（hashicorp/mdns）
- Controller mDNS 监听（hashicorp/mdns，注意 Windows 兼容性）
- Controller TaskExecutionClient（调用 Agent 的 Execute 方法）
- Dashboard 自动刷新设备列表（React Query refetchInterval）
- 设备详情面板 + 历史负载曲线

## LocalPilot 是什么

一个**个人设备网格计算平台**——把吃灰的旧电脑、旧手机变成你的私有计算集群。用户提交任务，系统自动发现设备、跨设备调度工作、在沙箱中执行，并在节点故障时自动迁移任务。

## 项目编写必须遵守的需求
每次编写的时候对每个函数，每个文件进行详细的注释
每个架构，都在当前文件夹下下面创建一个子claude.md，并在当前md创建索引
每次写的时候都要问自己，这个思路和代码有必要吗，为什么要这么做


## 架构（3 个组件）

| 组件 | 语言 | 职责 |
|------|------|------|
| **Agent（工作节点）** | Go | 运行在每台工作设备上。mDNS 广播、gRPC 心跳、沙箱中执行任务（Docker 或进程级隔离）、系统指标采集、日志流式推送。 |
| **Controller（控制中心）** | Go | 中央协调器。mDNS 监听器发现设备、设备注册表（内存 + SQLite）、多维任务调度器（CPU/内存/架构/延迟/GPU 打分）、REST API（gin）、WebSocket 实时状态推送、故障检测与任务迁移。 |
| **Dashboard（仪表板）** | React + TypeScript | Web 前端。实时设备状态网格、任务提交、终端日志查看器（ANSI 渲染）、集群拓扑图（React Flow）、指标仪表板。 |

### 前后端划分

| 角色 | 组件 | 语言 | 运行位置 |
|------|------|------|----------|
| **前端** | Dashboard | React + TypeScript | 用户浏览器 |
| **后端-控制面** | Controller | Go | 主力机（日常使用的设备） |
| **后端-执行面** | Agent | Go | 每台计算设备（旧 MacBook、旧笔记本等） |

- Dashboard ↔ Controller：**HTTP/2 + WebSocket**，典型前后端通信，用户直接感知
- Controller ↔ Agent：**gRPC**，后端内部服务间通信，用户不可见。注意：此 Agent 是分布式系统术语（系统守护进程），**非 LLM/AI Agent**

**服务发现**：mDNS（零配置局域网发现，`_localpilot._tcp`）。

## 目录结构

```
LocalPilot/
├── CLAUDE.md                     ← 你在读这个
├── LocalPilot-规划.md             — 原始设计文档（权威参考）
├── docs/
│   └── 学习清单.md                — 技术学习路线（含链接）
├── proto/                        — Protobuf IDL（系统契约）
│   ├── CLAUDE.md
│   └── localpilot/v1/
│       ├── agent.proto           — 设备注册/心跳/任务执行
│       ├── task.proto            — 任务模型/状态机/分片
│       └── discovery.proto       — 服务发现
├── pkg/proto/                    — 共享的 proto 生成 Go 代码
│   ├── go.mod
│   └── localpilot/v1/
│       ├── agent.pb.go / agent_grpc.pb.go
│       ├── task.pb.go
│       └── discovery.pb.go / discovery_grpc.pb.go
├── agent/                        — Go Agent
│   ├── CLAUDE.md
│   ├── go.mod / go.sum
│   ├── cmd/agent/main.go
│   └── internal/
│       ├── config/               — 配置管理
│       ├── monitor/              — 系统监控（跨平台）
│       ├── transport/            — gRPC client + server
│       ├── heartbeat/            — 心跳循环
│       ├── discovery/            — mDNS 广播 (Phase 1)
│       ├── executor/             — 任务执行器 (Phase 2)
│       └── sandbox/              — 沙箱 (Phase 2)
├── controller/                   — Go Controller
│   ├── CLAUDE.md
│   ├── go.mod / go.sum
│   ├── cmd/controller/main.go
│   └── internal/
│       ├── api/                  — HTTP API (gin)
│       ├── registry/             — 设备注册表 + 心跳检测
│       ├── scheduler/            — 调度器 (Phase 3)
│       ├── job/                  — 任务管理 (Phase 2)
│       ├── transport/            — gRPC 服务器
│       └── discovery/            — mDNS (Phase 1)
└── dashboard/                    — React 前端
    ├── CLAUDE.md
    ├── package.json
    ├── vite.config.ts
    └── src/
        ├── App.tsx / main.tsx
        ├── api/client.ts
        ├── types/index.ts
        ├── hooks/
        └── components/
```

## 关键设计决策

- **为什么 Agent 和 Controller 都用 Go？** 统一语言栈，降低认知负担和维护成本。共享 proto 生成的 Go 代码（`pkg/proto/`），通过 `replace` 指令引用。Go 交叉编译一样简单（`GOOS=linux GOARCH=arm64 go build`），goroutine 模型天然适合心跳循环和 gRPC 并发。
- **SQLite（WAL 模式）** 而非 Postgres/MySQL：零运维、内嵌式，足以支撑数百台设备规模。
- **沙箱分层降级**：Docker → Linux cgroups + namespaces → macOS sandbox-exec，自动回退。
- **调度器打分模型**：`CPU × 0.35 + 内存 × 0.25 + 架构匹配 × 0.20 + 延迟 × 0.10 + GPU × 0.10`
- **心跳机制**：每 5 秒一次；15 秒无心跳 → UNHEALTHY；30 秒无心跳 → OFFLINE → 触发任务迁移。

## 实施计划（20 周，5 个阶段）

| 阶段 | 周数 | 目标 |
|------|------|------|
| 0 — 原型验证 | 1-2 | 验证 Go↔Go gRPC 通信 + mDNS 发现链路可行 |
| 1 — 设备网格底座 | 3-6 | 多设备发现、注册、心跳、Dashboard 设备网格 |
| 2 — 任务执行引擎 | 7-10 | 任务模型、作业队列、沙箱执行、实时日志流 |
| 3 — 智能调度 | 11-14 | 多维打分调度器、任务分片（RANGE/CHUNK/PARAM） |
| 4 — 容错与韧性 | 15-17 | 心跳故障检测、自动任务迁移、断点恢复 |
| 5 — 打磨 | 18-20 | Dashboard 可视化、演示视频、文档 |

## 开发记录

| 文档 | 内容 |
|------|------|
| [Agent-Go语言选择与实现记录](docs/Agent-Go语言选择与实现记录.md) | Agent 为什么选 Go、每个模块的设计思路、遇到的问题和解决过程 |

## 使用规划文档

`LocalPilot-规划.md` 是权威参考。实施时请遵循其中的设计决策。关键章节：

- §二 — 系统架构与数据流
- §三 — 技术选型与理由（含库版本）
- §四 — 各阶段详细实施计划
- §五 — 用于验证的演示场景
- §六 — 面试话术（项目动机与设计理由）
- §七 — 技术风险与对策
- §十 — 预期刁钻问题及回答

## 子组件文档索引

每个组件目录下都有独立的 `CLAUDE.md`，描述该组件的模块结构、设计决策和运行方式：

| 组件 | 文档 | 一句话 |
|------|------|--------|
| **Proto** | [proto/CLAUDE.md](proto/CLAUDE.md) | Protobuf 定义——系统的单一事实来源 |
| **Agent** | [agent/CLAUDE.md](agent/CLAUDE.md) | Go 工作节点守护进程 |
| **Controller** | [controller/CLAUDE.md](controller/CLAUDE.md) | Go 中央协调器 |
| **Dashboard** | [dashboard/CLAUDE.md](dashboard/CLAUDE.md) | React 前端仪表板 |
