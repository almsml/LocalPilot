// ============================================================
// mdns.go — mDNS 监听
//
// Phase 1 实现。
// 监听局域网内的 _localpilot._tcp 广播，发现新 Agent 后自动注册。
//
// Windows 注意事项：
//   hashicorp/mdns 在 Windows 上有已知问题（Issue #80）
//   - IPv6 多播 setsockopt 报错
//   - 防火墙默认拦截 UDP 5353
//   Phase 0 验证时需先确认 Windows 上 mDNS 是否可用
// ============================================================

package discovery

import "github.com/localpilot/controller/internal/registry"

// ListenForAgents 启动 mDNS 监听，发现新 Agent 后注册到 registry
//
// Phase 1 实现。
func ListenForAgents(reg *registry.DeviceRegistry) error {
	// TODO(Phase 1):
	//   1. 创建 mdns.Server，监听 _localpilot._tcp
	//   2. 发现新服务时解析 IP:Port
	//   3. 通过 gRPC 回连 Agent 的 DeviceService
	//   4. 将 Agent 注册到 registry
	_ = reg // 占位
	return nil
}
