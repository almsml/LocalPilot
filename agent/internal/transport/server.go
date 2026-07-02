// ============================================================
// server.go — gRPC 服务端（Controller → Agent）
//
// Agent 同时也是 gRPC server——Controller 通过这个服务
// 向 Agent 下发任务（Execute）和取消任务（CancelTask）。
//
// 当前 Phase 0：骨架实现，Execute 返回空 stream。
// Phase 2 会接入真实的 executor 模块。
//
// 与 Rust 版本 (agent/src/transport.rs TaskExecutionServiceImpl) 对应。
// ============================================================

package transport

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	pb "github.com/localpilot/proto/localpilot/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// ============================================================
// TaskExecutionServer — Agent 侧的任务执行 gRPC 服务
//
// Controller 调用 Agent 的 Execute 方法下发任务。
// Agent 通过 server-streaming 实时推送执行日志。
// ============================================================

// TaskExecutionServer 封装 Agent 侧的 gRPC 任务执行服务
type TaskExecutionServer struct {
	server *grpc.Server
	port   uint16
}

// taskExecutionServiceImpl 实现 TaskExecutionServiceServer 接口
//
// 当前 Phase 0：骨架实现，所有方法返回占位响应。
// Phase 2 接入 real executor。
type taskExecutionServiceImpl struct {
	pb.UnimplementedTaskExecutionServiceServer
}

// StartTaskServer 启动 Agent 侧的 gRPC 任务执行服务
//
// 这个函数在 main.go 的最后调用，会阻塞当前 goroutine 直到服务停止。
// 与 Rust 版本 start_task_server() 对应。
func StartTaskServer(port uint16) (*TaskExecutionServer, error) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("gRPC 任务服务监听失败 %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	impl := &taskExecutionServiceImpl{}

	// 注册 TaskExecutionService 实现
	pb.RegisterTaskExecutionServiceServer(grpcServer, impl)

	// 注册 gRPC 反射服务（方便用 grpcurl 调试）
	reflection.Register(grpcServer)

	slog.Info("gRPC 任务执行服务启动", "addr", addr)

	// 在后台 goroutine 中 serve
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC 任务服务异常退出", "error", err)
		}
	}()

	return &TaskExecutionServer{
		server: grpcServer,
		port:   port,
	}, nil
}

// Stop 优雅关闭 gRPC 服务
func (s *TaskExecutionServer) Stop() {
	slog.Info("gRPC 任务服务正在关闭...")
	s.server.GracefulStop()
	slog.Info("gRPC 任务服务已关闭")
}

// ============================================================
// Execute — Controller 下发任务（server-streaming）
//
// Controller 发送一个 ExecuteRequest，Agent 返回一个 LogChunk stream。
// 为什么是 server-streaming？
//   Agent 执行命令的同时需要实时推送日志到 Controller，
//   而不是等执行完一次性返回。Controller 再把日志通过 WebSocket
//   推到 Dashboard，用户在浏览器里看到实时滚动。
// ============================================================

// Execute 处理任务执行请求
//
// Phase 0: 返回空 stream——只验证 gRPC 链路。
// Phase 2: 接入 executor 模块，实时推送执行日志。
//
// 与 Rust 版本 execute() 对应。
func (s *taskExecutionServiceImpl) Execute(
	req *pb.ExecuteRequest,
	stream pb.TaskExecutionService_ExecuteServer,
) error {
	slog.Info("收到任务执行请求",
		"task_id", req.TaskId,
		"command", req.Command,
	)

	// Phase 0: 不实际执行任何命令，只发送一条确认日志后关闭 stream
	logChunk := &pb.LogChunk{
		TaskId:    req.TaskId,
		StreamType: pb.StreamType_STREAM_TYPE_STDOUT,
		Data:      []byte("[Phase 0] 任务已接收，Agent 骨架就绪\n"),
		SeqNum:    0,
	}

	if err := stream.Send(logChunk); err != nil {
		slog.Error("发送日志 chunk 失败", "task_id", req.TaskId, "error", err)
		return err
	}

	slog.Info("任务执行请求处理完成（Phase 0 不实际执行）", "task_id", req.TaskId)
	return nil
}

// CancelTask 取消正在执行的任务
//
// Phase 0: 骨架——直接返回接受。
// Phase 2: 调用 executor 中止对应进程。
//
// 与 Rust 版本 cancel_task() 对应。
func (s *taskExecutionServiceImpl) CancelTask(
	ctx context.Context,
	req *pb.CancelTaskRequest,
) (*pb.CancelTaskResponse, error) {
	slog.Info("收到取消任务请求",
		"task_id", req.TaskId,
		"reason", req.Reason,
	)

	// Phase 0: 直接接受
	return &pb.CancelTaskResponse{
		Accepted: true,
	}, nil
}
