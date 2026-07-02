// ============================================================
// grpc.go — Controller 的 gRPC 服务器实现
//
// 实现 DeviceService（来自 agent.proto）：
//   - Register:  Agent 启动时调用，注册设备到集群
//   - Heartbeat: Agent 每 5 秒上报一次状态
//   - Deregister: Agent 正常退出时调用
//
// Controller 同时是 gRPC server（接收 Agent 请求）
// 和 gRPC client（调用 Agent 的 Execute 方法下发任务）。
// ============================================================

package transport

import (
	"context"
	"fmt"
	"log"
	"net"

	pb "github.com/localpilot/proto/localpilot/v1"
	"github.com/localpilot/controller/internal/registry"
	"github.com/localpilot/controller/internal/scheduler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// GRPCServer 封装 Controller 的 gRPC 服务
type GRPCServer struct {
	server    *grpc.Server
	registry  *registry.DeviceRegistry
	scheduler *scheduler.Scheduler
}

// NewGRPCServer 创建 gRPC 服务器
func NewGRPCServer(reg *registry.DeviceRegistry, sch *scheduler.Scheduler) *GRPCServer {
	s := &GRPCServer{
		registry:  reg,
		scheduler: sch,
	}

	grpcServer := grpc.NewServer()
	s.server = grpcServer

	// 注册 DeviceService 实现——Agent 通过这个服务注册/心跳/注销
	pb.RegisterDeviceServiceServer(grpcServer, &deviceServiceServer{registry: reg})

	// 注册 gRPC 反射服务（方便用 grpcurl 调试）
	reflection.Register(grpcServer)

	return s
}

// Serve 启动 gRPC 服务器
func (s *GRPCServer) Serve(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gRPC 监听失败 %s: %w", addr, err)
	}
	log.Printf("gRPC 服务启动: %s", addr)
	return s.server.Serve(lis)
}

// Stop 优雅关闭
func (s *GRPCServer) Stop(ctx context.Context) {
	log.Println("gRPC 服务正在关闭...")
	done := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		log.Println("gRPC 服务已关闭")
	case <-ctx.Done():
		log.Println("gRPC 强制关闭")
		s.server.Stop()
	}
}

// ============================================================
// deviceServiceServer — DeviceService gRPC 实现
//
// 这是 proto 中 DeviceService 的具体实现。
// 每个 RPC 方法接收 Agent 的请求，操作 DeviceRegistry。
// ============================================================

type deviceServiceServer struct {
	pb.UnimplementedDeviceServiceServer
	registry *registry.DeviceRegistry
}

// Register: Agent 上线 → 注册到集群
//
// Agent 启动后调用此方法，向 Controller 宣告自己的存在和能力。
// Controller 将设备信息存入 DeviceRegistry，返回分配的设备 ID。
func (s *deviceServiceServer) Register(
	ctx context.Context,
	req *pb.RegisterRequest,
) (*pb.RegisterResponse, error) {
	info := req.GetDeviceInfo()
	if info == nil {
		return nil, fmt.Errorf("device_info 不能为空")
	}

	// 构造设备对象
	device := &registry.Device{
		Hostname:          info.GetHostname(),
		OS:                info.GetOs(),
		Arch:              info.GetArch(),
		CPUCores:          info.GetCpuCores(),
		TotalRAMBytes:     info.GetTotalRamBytes(),
		GPUInfo:           info.GetGpuInfo(),
		SupportedRuntimes: info.GetSupportedRuntimes(),
		AgentAddress:      fmt.Sprintf("%s:%d", extractIP(ctx), 50052), // Agent gRPC 默认端口
	}

	// 注册到 DeviceRegistry
	if err := s.registry.Register(device); err != nil {
		return nil, fmt.Errorf("注册设备失败: %w", err)
	}

	log.Printf("[gRPC] 设备注册成功: hostname=%s id=%s arch=%s cores=%d",
		device.Hostname, device.ID, device.Arch, device.CPUCores)

	return &pb.RegisterResponse{
		DeviceId:             device.ID,
		HeartbeatIntervalSec: 5, // 心跳间隔：5 秒
	}, nil
}

// Heartbeat: Agent 定期上报状态
//
// Agent 每 5 秒调用一次，携带当前 CPU/内存/温度等指标。
// Controller 更新心跳时间戳——超时检测在 health.go 中独立运行。
func (s *deviceServiceServer) Heartbeat(
	ctx context.Context,
	req *pb.HeartbeatRequest,
) (*pb.HeartbeatResponse, error) {
	usage := req.GetResourceUsage()

	_, err := s.registry.UpdateHeartbeat(
		req.GetDeviceId(),
		float64(usage.GetCpuPercent()),
		usage.GetUsedRamBytes(),
		float64(usage.GetCpuTemperatureCelsius()),
		usage.GetRunningTaskCount(),
	)
	if err != nil {
		return nil, fmt.Errorf("心跳更新失败: %w", err)
	}

	return &pb.HeartbeatResponse{}, nil
}

// Deregister: Agent 正常退出
//
// 从注册表中移除设备。
func (s *deviceServiceServer) Deregister(
	ctx context.Context,
	req *pb.DeregisterRequest,
) (*pb.DeregisterResponse, error) {
	log.Printf("[gRPC] 设备注销: id=%s reason=%s", req.GetDeviceId(), req.GetReason())

	err := s.registry.Deregister(req.GetDeviceId())
	if err != nil {
		return &pb.DeregisterResponse{Accepted: false}, err
	}

	return &pb.DeregisterResponse{Accepted: true}, nil
}

// extractIP 从 gRPC context 中提取客户端 IP
func extractIP(ctx context.Context) string {
	// gRPC 的 peer 信息可以从 metadata 中提取
	// 简化版：返回 upstream 的连接地址
	return "127.0.0.1" // Phase 0: 本地测试
}
