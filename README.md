# LocalPilot

**个人设备网格计算平台** —— 把吃灰的旧电脑、旧手机变成你的私有计算集群。

[![Phase](https://img.shields.io/badge/phase-0%20原型验证-green)](#开发进度)
[![Go](https://img.shields.io/badge/Go-1.26.3-00ADD8?logo=go)](https://go.dev/)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react)](https://react.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.5-3178C6?logo=typescript)](https://www.typescriptlang.org/)
[![gRPC](https://img.shields.io/badge/gRPC-protocol-244c5a?logo=grpc)](https://grpc.io/)

---

## 这是什么

你有几台旧设备——一台旧 MacBook、一部旧手机——平时吃灰，偶尔需要算力时又不够用。LocalPilot 把它们组成一个**私有计算网格**：

- 🔍 **自动发现** — mDNS 零配置发现局域网内的计算设备
- 📊 **实时监控** — Dashboard 看 CPU/内存/温度，设备上下线实时推送
- 📦 **任务分发** — 提交计算任务，系统自动调度到最合适的设备
- 🛡️ **沙箱执行** — Docker 或进程级隔离，安全运行不受信任的代码
- 🔄 **故障迁移** — 节点挂了自动把任务迁移到其他设备

## 架构

```
┌──────────────────────────────────────────────────────┐
│                   Dashboard (React)                   │
│              浏览器 · HTTP/2 + WebSocket              │
└──────────────────────┬───────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────┐
│                Controller (Go · 主力机)               │
│  设备注册 · 心跳检测 · 任务调度 · REST API · WS 推送   │
└──────────────────────┬───────────────────────────────┘
                       │ gRPC
          ┌────────────┼────────────┐
          │            │            │
┌─────────▼──┐  ┌─────▼─────┐  ┌──▼──────────┐
│ Agent (Go) │  │ Agent (Go)│  │ Agent (Go)   │
│ 旧 MacBook  │  │  旧笔记本   │  │  旧手机       │
│ x86_64     │  │  x86_64   │  │  ARM64       │
└────────────┘  └───────────┘  └──────────────┘
```

| 组件 | 语言 | 职责 |
|------|------|------|
| **Agent** | Go | 工作在每台计算设备上。mDNS 广播、gRPC 心跳、沙箱中执行任务、系统指标采集。 |
| **Controller** | Go | 中央协调器。设备发现与注册、多维任务调度、REST API、WebSocket 推送。 |
| **Dashboard** | React + TypeScript | Web 前端。设备状态网格、任务提交、终端日志查看器、集群拓扑图。 |

## 快速开始

> 前置条件：Go 1.26+、Node.js 18+、protoc（仅开发需要）

```bash
# 1. 克隆仓库
git clone https://github.com/your-org/localpilot.git
cd LocalPilot

# 2. 启动 Controller（终端 1）
cd controller
go run ./cmd/controller/

# 3. 启动 Agent（终端 2）
cd agent
LOCALPILOT_CONTROLLER_HOST=127.0.0.1 go run ./cmd/agent/

# 4. 启动 Dashboard（终端 3）
cd dashboard
npm install
npm run dev

# 5. 浏览器打开 http://localhost:5173
```

## 目录结构

```
LocalPilot/
├── README.md                   ← 你在读这个
├── CLAUDE.md                   — Claude Code 项目指南
├── LocalPilot-规划.md           — 原始设计文档（权威参考）
├── docs/                       — 技术学习路线、开发记录
├── proto/                      — Protobuf IDL（系统契约）
│   └── localpilot/v1/
│       ├── agent.proto         — 设备注册/心跳/任务执行
│       ├── task.proto          — 任务模型/状态机/分片
│       └── discovery.proto     — 服务发现
├── pkg/proto/                  — proto 生成的 Go 代码（共享）
├── agent/                      — Go Agent（工作节点）
│   ├── cmd/agent/main.go
│   └── internal/
│       ├── config/             — 配置管理
│       ├── monitor/            — 系统监控（跨平台）
│       ├── transport/          — gRPC client + server
│       ├── heartbeat/          — 心跳循环
│       ├── discovery/          — mDNS 广播 (Phase 1)
│       ├── executor/           — 任务执行器 (Phase 2)
│       └── sandbox/            — 沙箱 (Phase 2)
├── controller/                 — Go Controller（控制中心）
│   ├── cmd/controller/main.go
│   └── internal/
│       ├── api/                — HTTP API (gin)
│       ├── registry/           — 设备注册表 + 心跳检测
│       ├── transport/          — gRPC 服务器
│       ├── scheduler/          — 调度器 (Phase 3)
│       ├── job/                — 任务管理 (Phase 2)
│       └── discovery/          — mDNS (Phase 1)
└── dashboard/                  — React 前端仪表板
    └── src/
        ├── api/client.ts       — API 客户端
        ├── types/index.ts      — TypeScript 类型
        ├── hooks/              — React Query Hooks
        └── components/         — UI 组件
```

## 开发进度

| 阶段 | 周数 | 目标 | 状态 |
|------|------|------|------|
| 0 — 原型验证 | 1-2 | gRPC 通信 + mDNS 发现链路验证 | ✅ 完成 |
| 1 — 设备网格底座 | 3-6 | 多设备发现、注册、心跳、Dashboard 实时刷新 | 🚧 进行中 |
| 2 — 任务执行引擎 | 7-10 | 任务模型、作业队列、沙箱执行、实时日志流 | ⏳ 待开始 |
| 3 — 智能调度 | 11-14 | 多维打分调度器、任务分片 | ⏳ 待开始 |
| 4 — 容错与韧性 | 15-17 | 故障检测、自动任务迁移、断点恢复 | ⏳ 待开始 |
| 5 — 打磨 | 18-20 | 可视化、演示视频、文档 | ⏳ 待开始 |

### Phase 0 已完成功能

- ✅ Proto 定义（agent / task / discovery）
- ✅ Go Agent 项目骨架（9 个源文件，编译通过）
- ✅ Go Controller 项目骨架（12 个源文件，编译通过）
- ✅ React Dashboard 项目骨架（10 个源文件，编译通过）
- ✅ Agent ↔ Controller gRPC 互通（Register / Heartbeat / Deregister）
- ✅ 心跳循环（每 5 秒）
- ✅ HTTP API `GET /api/devices` 返回设备数据
- ✅ Dashboard 设备卡片 UI

## 关键设计决策

- **为什么 Agent 和 Controller 都用 Go？** 统一语言栈，降低认知负担。Go 交叉编译简单（`GOOS=linux GOARCH=arm64 go build`），goroutine 模型天然适合心跳循环和 gRPC 并发。
- **为什么 SQLite 而不是 Postgres？** 零运维、内嵌式，足以支撑数百台设备。WAL 模式保证并发读写性能。
- **沙箱分层降级** — Docker → Linux cgroups+namespaces → macOS sandbox-exec，不可用时自动回退，不阻塞任务执行。
- **调度器打分模型** — `CPU×0.35 + 内存×0.25 + 架构匹配×0.20 + 延迟×0.10 + GPU×0.10`，多维度综合评分。
- **心跳机制** — 每 5 秒一次；15 秒无心跳 → UNHEALTHY；30 秒无心跳 → OFFLINE → 触发任务迁移。

## 技术栈

| 层 | 技术 |
|----|------|
| 服务间通信 | gRPC + Protobuf |
| 前端通信 | HTTP/2 + WebSocket |
| 服务发现 | mDNS（`_localpilot._tcp`） |
| Agent/Controller | Go 1.26 |
| Dashboard | React 18 + TypeScript 5 + Vite 5 |
| 状态管理 | React Query (TanStack Query) |
| UI 组件 | React Flow（拓扑图）+ Recharts（图表）+ react-hot-toast |
| 数据库 | SQLite（WAL 模式，modernc.org/sqlite 纯 Go 驱动） |
| 容器化 | Docker SDK for Go |

## 文档

| 文档 | 内容 |
|------|------|
| [LocalPilot-规划.md](LocalPilot-规划.md) | 完整设计文档（权威参考） |
| [CLAUDE.md](CLAUDE.md) | Claude Code 项目指南 |
| [docs/学习清单.md](docs/学习清单.md) | 技术学习路线 |
| [docs/Agent-Go语言选择与实现记录.md](docs/Agent-Go语言选择与实现记录.md) | Agent 设计记录 |
| [proto/CLAUDE.md](proto/CLAUDE.md) | Protobuf 定义说明 |
| [agent/CLAUDE.md](agent/CLAUDE.md) | Agent 组件文档 |
| [controller/CLAUDE.md](controller/CLAUDE.md) | Controller 组件文档 |
| [dashboard/CLAUDE.md](dashboard/CLAUDE.md) | Dashboard 组件文档 |

## License

MIT
