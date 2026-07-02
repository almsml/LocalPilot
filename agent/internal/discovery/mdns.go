// ============================================================
// mdns.go — Agent 侧 mDNS 广播
//
// Agent 启动后通过 mDNS 协议在局域网广播自己的存在，
// Controller 可以自动发现新设备而无需手动配置 IP。
//
// 为什么选择 mDNS？
//   零配置局域网服务发现——不需要 DNS 服务器、不需要手动注册。
//   标准协议（RFC 6762），macOS Bonjour / Linux Avahi / Windows 10+
//   都原生支持 mDNS 查询。
//
// 为什么用 hashicorp/mdns？
//   纯 Go 实现，无 CGO，交叉编译简单。
//   同时支持 Server 模式（广播服务）和 Lookup 模式（发现服务）。
//
// Windows 兼容性：
//   hashicorp/mdns 在 Windows 上可能存在多播绑定问题
//   （GitHub Issue #80）。mDNS 失败不阻塞 Agent 启动——
//   Controller 仍可通过直接 IP 方式连接。
// ============================================================

package discovery

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"

	"github.com/hashicorp/mdns"

	"github.com/localpilot/agent/internal/config"
)

// ============================================================
// StartMDNSBroadcast 启动 mDNS 广播
//
// 将 Agent 的 gRPC 服务注册为 _localpilot._tcp 类型的
// mDNS 服务。Controller 的 mDNS 监听器会检测到此广播并
// 记录设备信息。
//
// 参数：
//   - cfg: Agent 配置（hostname、agent_port、mDNS 服务类型）
//
// 返回：
//   - *mdns.Server: mDNS 服务器句柄，调用者通过 Shutdown() 停止广播
//   - error: 启动失败时返回错误（非致命，调用者可忽略）
//
// TXT 记录包含：
//   - agent_port: Agent 的 gRPC 端口（Controller 通过此端口回连）
//   - hostname: 设备主机名（Dashboard 展示用）
//   - version: 协议版本（未来兼容性检查）
// ============================================================

// StartMDNSBroadcast 在局域网广播 Agent 的存在
func StartMDNSBroadcast(cfg *config.Config) (*mdns.Server, error) {
	// ---- 获取本机 IP ----
	// 优先选择非 loopback 的 IPv4 地址作为广播地址。
	// 为什么不用 net.InterfaceAddrs() 遍历所有接口？
	//   多网卡设备（WiFi + 以太网 + 虚拟网卡）可能返回多个 IP。
	//   取第一个非 loopback、非链路本地的 IPv4 地址作为服务地址——
	//   Controller 通过此 IP + agent_port 回连 Agent。
	hostIP, err := getLocalIP()
	if err != nil {
		slog.Warn("mDNS: 无法获取本机 IP，使用 127.0.0.1", "error", err)
		hostIP = net.ParseIP("127.0.0.1")
	}

	// ---- 获取设备名 ----
	// 优先用配置中的 hostname（可能来自 LOCALPILOT_HOSTNAME 环境变量），
	// 回退到系统 hostname。
	hostname := cfg.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	// ---- 构建 TXT 记录 ----
	// TXT 记录是 mDNS 的扩展字段，Controller 从中读取 Agent 的
	// gRPC 端口等元信息。DNS-SD 规范允许任意 key=value 对。
	txtRecords := []string{
		fmt.Sprintf("agent_port=%d", cfg.AgentPort),
		fmt.Sprintf("hostname=%s", hostname),
		"version=1.0",
	}

	// ---- 创建 mDNS 服务条目 ----
	// NewMDNSService 创建一个符合 RFC 6762 标准的服务描述。
	// 参数说明：
	//   instance: 设备名（如 "old-macbook"），在局域网内应唯一
	//   service:  服务类型（_localpilot._tcp）
	//   domain:   mDNS 域名（固定为 "local."）
	//   host:     主机名（留空让库自动解析）
	//   port:     服务端口（Agent 的 gRPC 端口）
	//   ips:      本机 IP 列表
	//   txt:      TXT 记录（元数据键值对）
	service, err := mdns.NewMDNSService(
		hostname,                // 实例名（设备名）
		cfg.MDNSServiceType,     // 服务类型: _localpilot._tcp
		"local.",                // mDNS 域名
		"",                      // 主机名（留空自动填充）
		int(cfg.AgentPort),      // 服务端口
		[]net.IP{hostIP},        // 本机 IP
		txtRecords,              // TXT 记录
	)
	if err != nil {
		return nil, fmt.Errorf("创建 mDNS 服务条目失败: %w", err)
	}

	// ---- Windows 兼容性警告 ----
	// hashicorp/mdns 在 Windows 上使用 IPv6 多播地址 [ff02::fb]:5353。
	// 部分 Windows 版本/网络配置不支持 IPv6 多播 setsockopt。
	// 如果启动失败，Agent 仍可通过直接 IP 连接 Controller。
	if runtime.GOOS == "windows" {
		slog.Warn("mDNS: Windows 上可能需要防火墙允许 UDP 5353 端口，" +
			"并将网络设为「专用网络」。如果 mDNS 启动失败，" +
			"Agent 仍可通过 LOCALPILOT_CONTROLLER_HOST 直接连接 Controller。")
	}

	// ---- 启动 mDNS 服务器 ----
	// NewServer 在后台 goroutine 中持续运行：
	//   - 定期广播服务（约 120 秒间隔）
	//   - 收到 mDNS 查询时立即单播响应
	//   - 收到冲突检测时触发回调
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		return nil, fmt.Errorf("mDNS 服务器启动失败: %w", err)
	}

	slog.Info("mDNS 广播已启动",
		"service", cfg.MDNSServiceType,
		"hostname", hostname,
		"port", cfg.AgentPort,
		"ip", hostIP.String(),
	)

	return server, nil
}

// ============================================================
// getLocalIP 获取本机首选局域网 IPv4 地址
//
// 遍历所有网络接口，返回第一个非 loopback、非链路本地的
// IPv4 地址。
//
// 为什么不用 net.Dial("udp", "8.8.8.8:80") 获取出口 IP？
//   net.Dial 虽然不实际发送数据，但依赖路由表查询。
//   遍历接口更直接，不依赖外部网络可达性，
//   且在纯局域网环境也能正常工作。
// ============================================================

// getLocalIP 返回本机首选局域网 IPv4 地址
func getLocalIP() (net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("枚举网络接口失败: %w", err)
	}

	for _, iface := range interfaces {
		// 跳过未启用的网络接口（Down 状态）
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		// 跳过 loopback 接口（127.0.0.1）
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			// 只取 IPv4 地址（IPv6 多播行为在各平台差异大）
			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			// 跳过链路本地地址（169.254.x.x）
			// 链路本地地址无法路由，Controller 无法回连
			if ip.IsLinkLocalUnicast() {
				continue
			}

			return ip, nil
		}
	}

	return nil, fmt.Errorf("未找到可用局域网 IPv4 地址")
}
