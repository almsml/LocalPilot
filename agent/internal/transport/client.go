// ============================================================
// client.go — gRPC 客户端（Agent → Controller）
//
// 封装 Agent 调用 Controller DeviceService 的客户端逻辑：
//   - Connect: 建立到 Controller 的 gRPC 连接
//   - Register: 向 Controller 注册本设备
//   - Heartbeat: 定期发送心跳
//   - Deregister: 正常退出时注销
//
// 为什么封装 gRPC 通信？
//   1. 隔离 proto 生成的代码和业务逻辑
//   2. 将来加入 mTLS/重连/负载均衡时只改这一个文件
//   3. 方便写单元测试——mock 这个接口即可
//
// 与 Rust 版本 (agent/src/transport.rs DeviceServiceClient) 对应。
// ============================================================

package transport

import (
	"context"
	"fmt"
	"log/slog"

	pb "github.com/localpilot/proto/localpilot/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ============================================================
// DeviceServiceClient — Agent 调用 Controller 的 gRPC 客户端
//
// 封装了 Register、Heartbeat、Deregister 三个 RPC。
// 与 Rust 版本 DeviceServiceClient 对应。
// ============================================================

// DeviceServiceClient Agent 侧的 Controller 通信客户端
//
// 线程安全：gRPC client 支持并发调用，可以在心跳 goroutine
// 和主 goroutine 中同时使用。
type DeviceServiceClient struct {
	conn   *grpc.ClientConn
	client pb.DeviceServiceClient
}

// Connect 建立到 Controller 的 gRPC 连接
//
// 当前 Phase 0: 明文 HTTP/2 连接（不加密）。
// Phase 4 会在这一步加入 mTLS。
//
// 与 Rust 版本 connect() 对应。
func Connect(ctx context.Context, host string, port uint16) (*DeviceServiceClient, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	slog.Info("正在连接 Controller gRPC", "addr", addr)

	// 明文连接（Phase 0），Phase 4 替换为 TLS credentials
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // 阻塞直到连接成功或超时
	)
	if err != nil {
		return nil, fmt.Errorf("连接 Controller 失败 (%s): %w", addr, err)
	}

	client := pb.NewDeviceServiceClient(conn)
	return &DeviceServiceClient{
		conn:   conn,
		client: client,
	}, nil
}

// Register 向 Controller 注册本设备
//
// Agent 启动后调用此方法，向 Controller 宣告自己的存在和能力。
// Controller 返回分配的 device_id 和期望的心跳间隔。
//
// 与 Rust 版本 register() 对应。
func (c *DeviceServiceClient) Register(
	ctx context.Context,
	deviceInfo *pb.DeviceInfo,
) (*pb.RegisterResponse, error) {
	slog.Info("正在注册设备", "hostname", deviceInfo.Hostname)

	resp, err := c.client.Register(ctx, &pb.RegisterRequest{
		DeviceInfo: deviceInfo,
	})
	if err != nil {
		return nil, fmt.Errorf("设备注册失败: %w", err)
	}

	slog.Info("设备注册成功",
		"device_id", resp.DeviceId,
		"heartbeat_interval_sec", resp.HeartbeatIntervalSec,
	)
	return resp, nil
}

// Heartbeat 发送心跳
//
// Agent 定时调用此方法，携带当前 CPU/内存/温度等指标。
// Controller 通过心跳判断设备是否存活。
//
// 与 Rust 版本 heartbeat() 对应。
func (c *DeviceServiceClient) Heartbeat(
	ctx context.Context,
	req *pb.HeartbeatRequest,
) (*pb.HeartbeatResponse, error) {
	resp, err := c.client.Heartbeat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("心跳发送失败 (device=%s): %w", req.DeviceId, err)
	}
	return resp, nil
}

// Deregister 向 Controller 注销本设备
//
// Agent 正常退出时调用，从集群中移除自己。
// 与 Rust 版本 deregister() 对应。
func (c *DeviceServiceClient) Deregister(
	ctx context.Context,
	deviceID string,
	reason string,
) error {
	slog.Info("正在注销设备", "device_id", deviceID, "reason", reason)

	_, err := c.client.Deregister(ctx, &pb.DeregisterRequest{
		DeviceId: deviceID,
		Reason:   reason,
	})
	if err != nil {
		return fmt.Errorf("设备注销失败: %w", err)
	}

	slog.Info("设备已注销", "device_id", deviceID)
	return nil
}

// Close 关闭 gRPC 连接
//
// Agent 退出时调用，释放连接资源。
func (c *DeviceServiceClient) Close() error {
	return c.conn.Close()
}
