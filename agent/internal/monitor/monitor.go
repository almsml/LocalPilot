// ============================================================
// monitor.go — 系统资源监控
//
// 为什么需要这个模块？
//   调度器需要知道每台设备的实时负载，才能做出正确的分配决策。
//   这个模块采集 CPU、内存、磁盘、温度等指标，供心跳上报。
//
// 跨平台策略：
//   - 优先使用 gopsutil（已安装 v4.26.6），提供精确实时指标
//   - gopsutil 不可用时降级到平台特定实现（见 monitor_windows.go）
//
// 与 Rust 版本 (agent/src/monitor.rs) 功能对应。
// ============================================================

package monitor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/sensors"

	pb "github.com/localpilot/proto/localpilot/v1"

	"github.com/localpilot/agent/internal/config"
)

// ============================================================
// 设备静态信息采集（注册时使用）
// ============================================================

// CollectDeviceInfo 采集设备的静态能力信息
//
// 这些信息在设备运行期间不会变化，只需要在注册时上报一次。
// 与 Rust 版本 collect_device_info() 对应。
func CollectDeviceInfo(cfg *config.Config) *pb.DeviceInfo {
	// 收集支持的运行时
	runtimes := []string{"process"} // 进程级执行总是可用

	// 检查 Docker 是否可用
	// 简单粗暴：看 /var/run/docker.sock（Linux/macOS）或 named pipe（Windows）是否存在
	if dockerAvailable() {
		runtimes = append(runtimes, "docker")
	}

	// 检测 GPU
	gpuInfo := detectGPU()

	// 总内存：优先用 gopsutil，失败时降级到平台特定实现
	totalRAM := getTotalRAM()

	return &pb.DeviceInfo{
		Hostname:          cfg.Hostname,
		Os:                runtime.GOOS,
		Arch:              runtime.GOARCH,
		CpuCores:          uint32(runtime.NumCPU()),
		TotalRamBytes:     totalRAM,
		GpuInfo:           gpuInfo,
		SupportedRuntimes: runtimes,
	}
}

// ============================================================
// 设备实时资源采集（心跳时使用）
// ============================================================

// CollectResourceUsage 采集当前资源使用情况
//
// 使用 gopsutil 获取精确的实时指标：
//   - CPU 使用率：cpu.Percent() 内部处理采样间隔
//   - 已用内存：mem.VirtualMemory().Used
//   - CPU 温度：host.SensorsTemperatures() 按平台自动选择传感器
//
// 与 Rust 版本 collect_resource_usage() 对应。
func CollectResourceUsage() *pb.ResourceUsage {
	// ---- CPU 使用率 ----
	// cpu.Percent 需要传入采样间隔。与 Rust sysinfo 不同，
	// gopsutil 的 Percent 内部会 sleep 等间隔再采样，
	// 第一次调用返回 0（需要两次采样之间的差值）。
	// 这里传 100ms，足够短不阻塞心跳。
	var cpuPercent float32
	if percents, err := cpu.Percent(100*time.Millisecond, false); err == nil && len(percents) > 0 {
		cpuPercent = float32(percents[0] / 100.0) // gopsutil 返回 0~100，转换为 0.0~1.0
	}

	// ---- 内存使用 ----
	var usedRAM uint64
	if vmem, err := mem.VirtualMemory(); err == nil {
		usedRAM = vmem.Used
	} else {
		// 降级到平台特定实现
		usedRAM = getUsedRAMFallback()
	}

	// ---- CPU 温度 ----
	// gopsutil 的 SensorsTemperatures 跨平台兼容。
	// 不可用时返回 -1（与 Rust 版本行为一致）。
	temperature := -1.0
	if temps, err := sensors.SensorsTemperatures(); err == nil {
		// 找第一个标记为 CPU 的传感器
		for _, t := range temps {
			if strings.Contains(strings.ToLower(t.SensorKey), "cpu") {
				temperature = t.Temperature
				break
			}
		}
		// 如果没找到 CPU 标签，取第一个传感器的温度
		if temperature == -1.0 && len(temps) > 0 {
			temperature = temps[0].Temperature
		}
	}

	return &pb.ResourceUsage{
		CpuPercent:            cpuPercent,
		UsedRamBytes:          usedRAM,
		UsedDiskBytes:         0, // Phase 0 暂不采集磁盘
		CpuTemperatureCelsius: float32(temperature),
		RunningTaskCount:      0, // 由 executor 模块更新
	}
}

// ============================================================
// Docker 检测
// ============================================================

// dockerAvailable 检查 Docker 是否可用
//
// Linux/macOS: 检查 /var/run/docker.sock
// Windows: 检查 named pipe \\.\pipe\docker_engine
func dockerAvailable() bool {
	if runtime.GOOS == "windows" {
		_, err := os.Stat(`\\.\pipe\docker_engine`)
		return err == nil
	}
	_, err := os.Stat("/var/run/docker.sock")
	return err == nil
}

// ============================================================
// GPU 检测
// ============================================================

// detectGPU 检测 GPU 信息
//
// 为什么需要 GPU 信息？
//   调度器的 GPU 维度打分需要知道设备有没有 GPU。
//   比如 AI 推理任务必须分配到有 GPU 的设备上。
//
// 与 Rust 版本 detect_gpu() 对应，使用相同的检测策略。
func detectGPU() string {
	switch runtime.GOOS {
	case "darwin":
		return detectGPUmacOS()
	case "linux":
		return detectGPULinux()
	case "windows":
		return detectGPUWindows()
	default:
		return ""
	}
}

// detectGPUmacOS macOS GPU 检测
func detectGPUmacOS() string {
	out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
	if err != nil {
		return "Apple Silicon (unknown)"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "Chipset Model:") {
			return strings.TrimSpace(strings.Replace(line, "Chipset Model:", "", 1))
		}
	}
	return "Apple Silicon (unknown)"
}

// detectGPULinux Linux GPU 检测
func detectGPULinux() string {
	if _, err := os.Stat("/proc/driver/nvidia"); err == nil {
		out, err := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
		return "NVIDIA GPU"
	}
	return ""
}

// detectGPUWindows Windows GPU 检测
func detectGPUWindows() string {
	// 使用 gopsutil 的信息或 wmic 命令
	return ""
}

// ============================================================
// 内存信息采集
//
// 优先使用 gopsutil（跨平台），失败时降级到平台特定实现。
// ============================================================

// getTotalRAM 获取系统总内存（字节）
//
// gopsutil 的 mem.VirtualMemory().Total 跨平台可用，
// Windows/Linux/macOS 都返回正确的物理内存总量。
func getTotalRAM() uint64 {
	if vmem, err := mem.VirtualMemory(); err == nil {
		return vmem.Total
	}
	// 降级到平台特定实现
	return getTotalRAMFallback()
}

// getUsedRAMFallback 平台特定的已用内存（gopsutil 不可用时降级）
func getUsedRAMFallback() uint64 {
	return getTotalRAMFallback() - getAvailableRAMFallback()
}

// getTotalRAMFallback 平台特定的总内存
func getTotalRAMFallback() uint64 {
	switch runtime.GOOS {
	case "windows":
		return getWindowsMemoryInfo().totalPhys
	case "linux":
		return parseProcMeminfo("MemTotal")
	case "darwin":
		return getTotalRAMmacOS()
	default:
		return 0
	}
}

// getAvailableRAMFallback 平台特定的可用内存
func getAvailableRAMFallback() uint64 {
	switch runtime.GOOS {
	case "windows":
		return getWindowsMemoryInfo().availPhys
	case "linux":
		return parseProcMeminfo("MemAvailable")
	case "darwin":
		return 0 // macOS 降级复杂，返回 0
	default:
		return 0
	}
}

// ============================================================
// Windows 内存（调度到 monitor_windows.go）
// ============================================================

// windowsMemoryInfo Windows 内存信息快照
type windowsMemoryInfo struct {
	totalPhys uint64
	availPhys uint64
}

// ============================================================
// Linux 内存实现（降级路径）
//
// 读取 /proc/meminfo 文件，解析 MemTotal 和 MemAvailable 字段。
// ============================================================

// getTotalRAMLinux 已废弃——保留用于降级路径
// getUsedRAMLinux 已废弃——保留用于降级路径

// parseProcMeminfo 从 /proc/meminfo 中提取指定字段的值
//
// 字段值以 kB 为单位，返回时转换为字节（×1024）。
func parseProcMeminfo(field string) uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, field+":") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				var kb uint64
				fmt.Sscanf(parts[1], "%d", &kb)
				return kb * 1024
			}
		}
	}
	return 0
}

// ============================================================
// macOS 内存实现（降级路径）
// ============================================================

// getTotalRAMmacOS macOS 总内存（降级路径）
func getTotalRAMmacOS() uint64 {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0
	}
	var bytes uint64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &bytes)
	return bytes
}
