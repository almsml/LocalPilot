// ============================================================
// client.go — Controller 侧的 gRPC 客户端（Controller → Agent）
//
// Controller 通过此客户端调用 Agent 的 TaskExecutionService：
//   - Execute: 下发任务到 Agent（server-streaming，接收实时日志）
//   - CancelTask: 取消 Agent 上正在执行的任务
//
// 为什么 Controller 同时是 gRPC server 和 gRPC client？
//   Agent 调用 Controller 的 DeviceService（注册/心跳/注销），
//   此时 Controller 是 server。
//   Controller 调用 Agent 的 TaskExecutionService（下发任务），
//   此时 Controller 是 client。
//   这个非对称设计反映了两个方向不同的通信需求——
//   控制面（注册/心跳）由 Agent 主动发起，数据面（任务下发）由 Controller 主动发起。
//
// Phase 1 状态：创建客户端对象但不实际调用 Execute。
//   调度器（Phase 3）完成后再集成此客户端。
// ============================================================

package transport

import (
	"context"
	"fmt"

	pb "github.com/localpilot/proto/localpilot/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TaskExecutionClient Controller 侧调用 Agent 的 gRPC 客户端
//
// 与 Agent 的 transport.DeviceServiceClient 模式一致——
// 封装 grpc.ClientConn + proto 生成的客户端 stub。
type TaskExecutionClient struct {
	conn   *grpc.ClientConn
	client pb.TaskExecutionServiceClient
}

// ConnectToAgent 建立到 Agent 的 gRPC 连接
//
// 参数：
//   - ctx: 上下文（用于超时控制）
//   - agentAddr: Agent 的 gRPC 地址，格式 "IP:Port"（如 "192.168.1.100:50052"）
//
// 为什么用 grpc.WithBlock()？
//   确保在函数返回时连接已经建立（或失败）。
//   非阻塞 Dial 可能在后台异步连接，后续 RPC 调用时才报错——
//   对于任务下发来说，我们希望在调度前就确认连通性。
func ConnectToAgent(ctx context.Context, agentAddr string) (*TaskExecutionClient, error) {
	conn, err := grpc.DialContext(ctx, agentAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("连接 Agent 失败 (%s): %w", agentAddr, err)
	}

	return &TaskExecutionClient{
		conn:   conn,
		client: pb.NewTaskExecutionServiceClient(conn),
	}, nil
}

// Execute 向 Agent 下发任务
//
// 返回一个 gRPC stream，Controller 通过这个 stream 接收 Agent
// 实时推送的执行日志（LogChunk）。
//
// Phase 2: 调度器将 Task 分配给 Agent 后调用此方法。
func (c *TaskExecutionClient) Execute(
	ctx context.Context,
	req *pb.ExecuteRequest,
) (pb.TaskExecutionService_ExecuteClient, error) {
	stream, err := c.client.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("下发任务失败: %w", err)
	}
	return stream, nil
}

// CancelTask 取消 Agent 上正在执行的任务
//
// Phase 2: 用户主动取消任务或故障迁移时调用。
func (c *TaskExecutionClient) CancelTask(
	ctx context.Context,
	taskID string,
	reason string,
) error {
	_, err := c.client.CancelTask(ctx, &pb.CancelTaskRequest{
		TaskId: taskID,
		Reason: reason,
	})
	if err != nil {
		return fmt.Errorf("取消任务失败 (task_id=%s): %w", taskID, err)
	}
	return nil
}

// Close 关闭与 Agent 的 gRPC 连接
//
// 为什么需要显式的 Close？
//   grpc.ClientConn 底层维护 HTTP/2 连接池。
//   不关闭会导致资源泄漏（文件描述符和内存）。
func (c *TaskExecutionClient) Close() error {
	return c.conn.Close()
}
