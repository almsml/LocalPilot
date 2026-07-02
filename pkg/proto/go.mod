// ============================================================
// pkg/proto/go.mod — 共享的 Protobuf 生成代码模块
//
// Agent 和 Controller 都通过 go.work 引用此模块，
// proto 定义是系统唯一的通信契约。
// ============================================================

module github.com/localpilot/proto

go 1.26.3

require (
	google.golang.org/grpc v1.82.0
	google.golang.org/protobuf v1.36.11
)

require (
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
)
