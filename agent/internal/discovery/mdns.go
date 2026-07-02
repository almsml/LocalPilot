// ============================================================
// mdns.go — mDNS 服务发现（广播端）
//
// Phase 1 实现。Phase 0 先用直接 IP 连接 Controller。
//
// 与 Rust 版本 (agent/src/discovery.rs) 对应。
// ============================================================

package discovery

// TODO(Phase 1): 使用 hashicorp/mdns 库广播 _localpilot._tcp 服务
// 让 Controller 能自动发现本 Agent，无需手动配置 IP。
//
// 实现要点：
//   1. 创建 mdns.Server，注册 _localpilot._tcp 服务
//   2. 广播本机的 Agent gRPC 端口（50052）和主机名
//   3. 监听 SIGTERM/SIGINT，优雅注销 mDNS 服务
