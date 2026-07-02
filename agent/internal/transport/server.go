// ============================================================
// server.go — gRPC 服务端（Controller → Agent）
//
// Agent 同时也是 gRPC server——Controller 通过这个服务
// 向 Agent 下发任务（Execute）和取消任务（CancelTask）。
//
// Phase 2: 接入 executor 模块，真实执行命令并流式推送日志。
// ============================================================

package transport

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	pb "github.com/localpilot/proto/localpilot/v1"
	"github.com/localpilot/agent/internal/executor"
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
type taskExecutionServiceImpl struct {
	pb.UnimplementedTaskExecutionServiceServer
	executor *executor.Executor // 任务执行器，负责实际执行命令
}

// StartTaskServer 启动 Agent 侧的 gRPC 任务执行服务
func StartTaskServer(port uint16, exec *executor.Executor) (*TaskExecutionServer, error) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("gRPC 任务服务监听失败 %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	impl := &taskExecutionServiceImpl{executor: exec}

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
// Controller 发送 ExecuteRequest，Agent 在沙箱中执行命令，
// 通过 server-streaming 实时推送 stdout/stderr 日志。
//
// 为什么是 server-streaming？
//   命令可能运行几分钟甚至几小时（如视频转码）。
//   server-streaming 让 Controller 和 Dashboard 能实时看到
//   命令输出——就像在终端里看 ping 逐行输出一样。
//
// 数据流：
//   ExecuteRequest → executor.Execute() → LogChunk channel
//     → stream.Send(logChunk) → Controller 接收
// ============================================================

// Execute 处理任务执行请求，实时流式推送日志
func (s *taskExecutionServiceImpl) Execute(
	req *pb.ExecuteRequest,
	stream pb.TaskExecutionService_ExecuteServer,
) error {
	slog.Info("收到任务执行请求",
		"task_id", req.GetTaskId(),
		"command", req.GetCommand(),
		"args", req.GetArgs(),
	)

	// 委托给 executor 执行
	result := s.executor.Execute(
		stream.Context(),
		req.GetTaskId(),
		req.GetCommand(),
		req.GetArgs(),
		req.GetEnv(),
		req.GetResourceLimits(),
	)

	// ---- 流式推送日志 ----
	// 从 LogChunks channel 逐条读取，通过 gRPC stream 发送给 Controller。
	// 为什么不用 for range channel？
	//   需要检查 stream.Context() 是否已取消——Controller 可能断开连接。
	for logChunk := range result.LogChunks {
		// 检查客户端是否还在连接
		if stream.Context().Err() != nil {
			slog.Warn("客户端已断开，停止推送日志", "task_id", req.GetTaskId())
			return stream.Context().Err()
		}

		if err := stream.Send(logChunk); err != nil {
			slog.Error("发送日志 chunk 失败", "task_id", req.GetTaskId(), "error", err)
			return fmt.Errorf("发送日志失败: %w", err)
		}
	}

	// ---- 等待任务完成 ----
	<-result.Done

	// 发送最终状态日志
	if result.Err != nil {
		finalChunk := &pb.LogChunk{
			TaskId:    req.GetTaskId(),
			StreamType: pb.StreamType_STREAM_TYPE_STDERR,
			Data:      []byte(fmt.Sprintf("[ERROR] 任务执行失败: %v\n", result.Err)),
		}
		if err := stream.Send(finalChunk); err != nil {
			return err
		}
		return result.Err
	}

	// 发送完成日志
	finalChunk := &pb.LogChunk{
		TaskId:    req.GetTaskId(),
		StreamType: pb.StreamType_STREAM_TYPE_STDOUT,
		Data:      []byte(fmt.Sprintf("[DONE] 任务完成，退出码: %d\n", result.Result.ExitCode)),
	}
	if err := stream.Send(finalChunk); err != nil {
		return err
	}

	slog.Info("任务执行完成并已推送全部日志",
		"task_id", req.GetTaskId(),
		"exit_code", result.Result.ExitCode,
	)

	return nil
}

// CancelTask 取消正在执行的任务
func (s *taskExecutionServiceImpl) CancelTask(
	ctx context.Context,
	req *pb.CancelTaskRequest,
) (*pb.CancelTaskResponse, error) {
	slog.Info("收到取消任务请求",
		"task_id", req.GetTaskId(),
		"reason", req.GetReason(),
	)

	err := s.executor.Cancel(req.GetTaskId())
	if err != nil {
		slog.Warn("取消任务失败", "task_id", req.GetTaskId(), "error", err)
		return &pb.CancelTaskResponse{Accepted: false}, err
	}

	return &pb.CancelTaskResponse{Accepted: true}, nil
}
