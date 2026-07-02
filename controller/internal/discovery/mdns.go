// ============================================================
// mdns.go — Controller 侧 mDNS 监听器
//
// Controller 在后台持续监听局域网内的 mDNS 广播，
// 发现新的 _localpilot._tcp 服务后记录日志。
//
// 为什么 Phase 1 只记录发现而不自动注册？
//   自动注册需要额外的安全考量：
//     1) 身份验证——不能信任任何广播自己是 Agent 的设备
//     2) 防重复——同一设备可能被 mDNS 和 gRPC 注册两次
//     3) gRPC 回连——需要验证 Agent 的 gRPC 端口确实可达
//   Phase 1 先验证 mDNS 发现链路可用。
//   Phase 2 会在发现后尝试 gRPC 回连验证，确认后再自动注册。
//
// Windows 兼容性：
//   hashicorp/mdns 在 Windows 上使用 IPv6 多播地址 [ff02::fb]:5353。
//   部分 Windows 版本/防火墙配置可能阻止多播。
//   如果 mDNS 不可用，设备发现回退到手动 IP 配置。
// ============================================================

package discovery

import (
	"log/slog"
	"runtime"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/localpilot/controller/internal/registry"
)

// ============================================================
// ListenForAgents 启动 mDNS 监听 goroutine
//
// 在后台持续扫描局域网内的 _localpilot._tcp 服务。
// 每 30 秒执行一次完整扫描（mdns.Query 内部有查询间隔优化）。
//
// 为什么是 30 秒扫描间隔？
//   mDNS 查询会根据网络规模自动调整。30 秒间隔确保
//   新设备能在半分钟内被发现，同时避免过度查询。
//
// 为什么 ListenForAgents 不返回 channel？
//   Phase 1 的发现结果只记录日志，不触发任何动作。
//   Phase 2 会加入事件 channel 通知 Controller 的其他模块。
// ============================================================

// ListenForAgents 启动 mDNS 监听
func ListenForAgents(reg *registry.DeviceRegistry) error {
	if runtime.GOOS == "windows" {
		slog.Warn("mDNS: Windows 上可能需要防火墙允许 UDP 5353 端口，" +
			"并将网络设为「专用网络」。如果 mDNS 启动失败，" +
			"设备发现将回退到手动 IP 配置。")
	}

	// entriesCh 接收发现的 mDNS 服务条目
	// 缓冲区大小 10 足以应对局域网内最多数十台设备的爆发发现
	entriesCh := make(chan *mdns.ServiceEntry, 10)

	// 启动 mDNS 查询 goroutine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("mDNS 监听 panic 恢复", "panic", r)
			}
		}()

		for {
			slog.Debug("mDNS: 开始扫描 _localpilot._tcp 服务...")

			// mdns.Query 发送 mDNS 查询并等待响应。
			// 查询参数：
			//   _localpilot._tcp: 服务类型（与 Agent 的广播一致）
			//   entriesCh: 结果通过此 channel 推送
			// 为什么用 channel 而不是 callback？
			//   channel 模式天然支持超时和优雅关闭——
			//   用 time.After 可以控制发现窗口，用 close(channel)
			//   可以停止发现，比 callback 更 Go idiomatic。
			queryParams := &mdns.QueryParam{
				Service:   "_localpilot._tcp",
				Domain:    "local.",
				Timeout:   5 * time.Second, // 单次查询超时
				Entries:   entriesCh,
				WantUnicastResponse: false,
			}

			if err := mdns.Query(queryParams); err != nil {
				slog.Warn("mDNS: 查询失败", "error", err)
				// 查询失败后等待一段时间再重试，避免频繁报错
				time.Sleep(30 * time.Second)
				continue
			}

			// 等待下一次扫描
			time.Sleep(30 * time.Second)
		}
	}()

	// 启动结果处理 goroutine
	go func() {
		for entry := range entriesCh {
			handleMDNSEntry(entry, reg)
		}
	}()

	slog.Info("mDNS 监听已启动",
		"service", "_localpilot._tcp",
		"note", "发现结果仅记录日志，自动注册将在 Phase 2 实现")

	return nil
}

// ============================================================
// handleMDNSEntry 处理发现的 mDNS 服务条目
//
// 解析 Agent 广播的 TXT 记录，提取端口和主机名信息，
// 记录日志以便调试和验证 mDNS 链路。
// ============================================================

// handleMDNSEntry 处理单个 mDNS 发现结果
func handleMDNSEntry(entry *mdns.ServiceEntry, reg *registry.DeviceRegistry) {
	// 提取 TXT 记录中的关键字段
	var agentPort string
	var hostname string

	for _, field := range entry.InfoFields {
		// TXT 记录格式: "key=value"
		// 为什么不用 strings.Cut？
		//   手动解析更清晰，且只有两个字段。
		for i := 0; i < len(field); i++ {
			if field[i] == '=' {
				key := field[:i]
				val := field[i+1:]
				switch key {
				case "agent_port":
					agentPort = val
				case "hostname":
					hostname = val
				}
				break
			}
		}
	}

	// 如果 TXT 记录中没有端口，使用 mDNS SRV 记录的端口
	if agentPort == "" {
		agentPort = "50052" // 默认 Agent gRPC 端口
	}

	if hostname == "" {
		hostname = entry.Host
	}

	// 构建 Agent 地址
	agentAddr := entry.AddrV4.String() + ":" + agentPort

	// ---- 检查是否已经注册 ----
	// 已注册设备不需要重复处理。
	// 先释放读锁再获取写锁是安全的，因为发现和注册是异步的。
	alreadyKnown := false
	for _, dev := range reg.ListDevices() {
		if dev.AgentAddress == agentAddr || dev.Hostname == hostname {
			alreadyKnown = true
			break
		}
	}

	if alreadyKnown {
		slog.Debug("mDNS: 已注册设备，跳过",
			"hostname", hostname,
			"addr", agentAddr,
		)
		return
	}

	slog.Info("mDNS 发现新 Agent",
		"hostname", hostname,
		"agent_addr", agentAddr,
		"note", "Phase 2 将实现自动注册",
	)
}
