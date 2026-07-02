# controller/ — Go Controller（中央协调器）

## 一句话

Controller 是 LocalPilot 的大脑：接收 Agent 注册和心跳、管理设备注册表、调度任务到最优设备、通过 HTTP API 和 WebSocket 为 Dashboard 提供实时数据。

## 模块地图

```
cmd/controller/main.go    — 入口：按顺序启动所有子系统
internal/
├── api/                  — HTTP API（gin）
│   ├── router.go         —   路由注册 + CORS 中间件
│   ├── device_handler.go —   GET /api/devices, GET /api/devices/:id
│   ├── job_handler.go    —   POST /api/jobs, GET /api/jobs/:id（Phase 2）
│   └── ws_handler.go     —   GET /ws WebSocket 连接（Phase 2）
├── registry/             — 设备注册表
│   ├── device.go         —   内存缓存 + SQLite 持久化 + 并发安全
│   └── health.go         —   心跳超时检测（15s UNHEALTHY, 30s OFFLINE）
├── scheduler/            — 任务调度器
│   ├── scheduler.go      —   调度器核心（Phase 3）
│   ├── scorer.go         —   多维打分引擎（Phase 3）
│   └── splitter.go       —   任务分片策略（Phase 3）
├── job/                  — 任务管理
│   ├── manager.go        —   生命周期管理（Phase 2）
│   ├── queue.go          —   任务队列（Phase 2）
│   └── store.go          —   SQLite 持久化（Phase 2）
├── transport/            — gRPC 客户端/服务器
│   └── grpc.go           —   Controller 作为 gRPC server（接收 Agent 注册/心跳）
└── discovery/            — 服务发现
    └── mdns.go           —   mDNS 监听器（Phase 1）
```

## 关键设计决策

1. **为什么设备注册表用 RWMutex 而不是 channel？**
   读操作（API 查询）远多于写操作（心跳更新）。RWMutex 允许多个读并发，写独占。Channel 模式需要额外的 goroutine 管理状态，在这里是过度设计。

2. **为什么分两层心跳检测（UNHEALTHY vs OFFLINE）？**
   避免网络抖动导致不必要的任务迁移。15s 标记 UNHEALTHY（给设备自救机会），30s 确认 OFFLINE（触发迁移）。

3. **为什么 API 用 gin 而不是标准库 net/http？**
   gin 的路由分组、中间件、JSON 绑定减少样板代码。对开发速度有帮助，且 gin 的性能完全满足需求。

## 编译与运行

```bash
# 编译
cd controller && go build ./cmd/controller

# 运行
go run ./cmd/controller

# 自定义端口
LOCALPILOT_GRPC_PORT=50051 LOCALPILOT_HTTP_PORT=8080 go run ./cmd/controller
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `LOCALPILOT_DB_PATH` | ./localpilot.db | SQLite 数据库路径 |
| `LOCALPILOT_GRPC_PORT` | 50051 | gRPC 监听端口 |
| `LOCALPILOT_HTTP_PORT` | 8080 | HTTP API 端口 |
| `LOCALPILOT_ENABLE_MDNS` | false | 是否启用 mDNS 监听（Phase 1） |

## 当前状态：Phase 0

- ✅ 项目骨架（go.mod + main.go）
- ✅ 设备注册表（内存 + SQLite、RWMutex 并发安全）
- ✅ 心跳超时检测（goroutine ticker）
- ✅ gRPC 服务器框架
- ✅ HTTP API 路由（设备列表/详情）
- ⏳ gRPC 服务实现（phase 1：proto 代码生成后接入）
- ⏳ 任务模型 + API（Phase 2）
- ⏳ mDNS 监听（Phase 1）
- ⏳ WebSocket 推送（Phase 2）
- ⏳ 调度器（Phase 3）
