// ============================================================
// agent/go.mod — LocalPilot Agent（Go 版本）
//
// Agent 运行在每台计算设备上，负责：
//   1. 向 Controller 注册并维持心跳
//   2. 接收任务并在沙箱中执行
//   3. 采集系统指标（CPU/内存/温度等）
//   4. mDNS 广播以被 Controller 自动发现
//
// 为什么 Agent 改用 Go？
//   统一语言栈，共享 proto 代码，简化工具链。
//   Go 交叉编译一样简单，goroutine 模型天然适合
//   心跳循环和 gRPC concurrency。
//
// 依赖策略：
//   - gopsutil v4.26.6 做跨平台系统监控（CPU/内存/温度）
//   - golang.org/x/sys 做 Windows 特定内存检测（降级路径）
//   - google.golang.org/grpc 做 gRPC 通信
// ============================================================

module github.com/localpilot/agent

go 1.26.3

require (
	github.com/localpilot/proto v0.0.0
	github.com/shirou/gopsutil/v4 v4.26.6
	golang.org/x/sys v0.44.0
	google.golang.org/grpc v1.82.0
)

require (
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/localpilot/proto => ../pkg/proto
