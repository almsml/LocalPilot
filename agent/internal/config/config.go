// ============================================================
// config.go — Agent 配置管理
//
// 为什么需要配置文件而不是硬编码？
//   每台设备的 hostname 不同、Controller 的 IP 也可能不同。
//   配置通过环境变量覆盖，方便 Docker 部署和脚本启动。
//
// 与 Rust 版本 (agent/src/config.rs) 的功能完全对应。
// ============================================================

package config

import (
	"fmt"
	"os"
	"runtime"
)

// ============================================================
// Config — Agent 配置结构体
//
// 所有字段都可以通过环境变量 LOCALPILOT_<FIELD> 覆盖，
// 优先级：环境变量 > 默认值
// ============================================================

// Config Agent 运行所需的所有配置项
type Config struct {
	// Hostname 本机标识名，默认读取系统 hostname
	Hostname string

	// ControllerHost Controller 的地址（IP 或域名）
	ControllerHost string

	// ControllerPort Controller 的 gRPC 端口
	ControllerPort uint16

	// AgentPort Agent 自己的 gRPC 监听端口
	AgentPort uint16

	// MDNSServiceType mDNS 服务名，固定为 _localpilot._tcp
	MDNSServiceType string
}

// Load 加载配置
//
// 当前 Phase 0 阶段：全部用默认值 + 环境变量覆盖。
// 将来可以从 YAML/TOML 文件加载。
func Load() (*Config, error) {
	hostname, err := getHostname()
	if err != nil {
		// hostname 获取失败时使用默认值，不阻塞启动
		hostname = "unknown-device"
	}

	cfg := &Config{
		Hostname:        hostname,
		ControllerHost:  getEnv("LOCALPILOT_CONTROLLER_HOST", "127.0.0.1"),
		ControllerPort:  getEnvUint16("LOCALPILOT_CONTROLLER_PORT", 50051),
		AgentPort:       getEnvUint16("LOCALPILOT_AGENT_PORT", 50052),
		MDNSServiceType: "_localpilot._tcp",
	}

	return cfg, nil
}

// ============================================================
// 辅助函数
// ============================================================

// getHostname 获取设备标识名
//
// 优先级：环境变量 LOCALPILOT_HOSTNAME > 系统 hostname
//
// 为什么需要 LOCALPILOT_HOSTNAME 环境变量？
//   Windows 的 os.Hostname() 有时返回被截断的 NETBIOS 名（最多 15 字符），
//   而不是完整的计算机名。Linux 容器中 hostname 常是随机容器 ID。
//   环境变量让用户可以指定一个人类可读的设备名，对 Dashboard 展示更友好。
//
// 跨平台实现：
//   - Linux/macOS: os.Hostname() 底层调用 gethostname(2)
//   - Windows: os.Hostname() 底层调用 GetComputerNameExW
func getHostname() (string, error) {
	// 优先使用用户显式指定的名称
	if name := os.Getenv("LOCALPILOT_HOSTNAME"); name != "" {
		return name, nil
	}

	name, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("获取 hostname 失败: %w", err)
	}
	return name, nil
}

// getEnv 读取环境变量，不存在时返回默认值
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getEnvUint16 读取环境变量并解析为 uint16，失败时返回默认值
func getEnvUint16(key string, defaultVal uint16) uint16 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	var parsed uint16
	if _, err := fmt.Sscanf(val, "%d", &parsed); err != nil {
		return defaultVal
	}
	return parsed
}

// String 返回配置的可读表示（隐藏敏感信息）
func (c *Config) String() string {
	return fmt.Sprintf(
		"hostname=%s controller=%s:%d agent_port=%d os=%s arch=%s",
		c.Hostname, c.ControllerHost, c.ControllerPort,
		c.AgentPort, runtime.GOOS, runtime.GOARCH,
	)
}
