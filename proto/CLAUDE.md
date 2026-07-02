# proto/ — Protobuf 定义（系统单一事实来源）

## 为什么 proto 在最外层？

所有组件（Agent、Controller）的通信协议由 proto 文件定义。
改任何字段都从这里开始，两边自动同步。proto 是契约——编译不过说明两边对不齐。

## 文件说明

| 文件 | 内容 | 依赖 |
|------|------|------|
| `agent.proto` | DeviceService（注册/心跳/注销）+ TaskExecutionService（执行/取消/日志流） | task.proto |
| `task.proto` | Task 模型、TaskStatus 状态机、TaskSharding 分片策略、Checkpoint | 无 |
| `discovery.proto` | 服务发现（当前只有 mDNS 配置常量，无 RPC） | 无 |

## 修改 proto 后的操作

```bash
# Rust Agent: 重新编译（build.rs 自动触发）
cd agent && cargo build

# Go Controller: 重新生成代码
cd controller
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       -I ../proto ../proto/localpilot/v1/*.proto
```

## 设计原则

- **proto3 语法** — 所有字段默认 optional（proto3 中 optional 是显式关键字）
- **字段编号** — 1-15 用 1 字节，留给最频繁使用的字段
- **package** — `localpilot.v1`，版本号在 package 中体现
- **不要删除字段** — 用 `reserved` 标记废弃字段，防止编号被重用
