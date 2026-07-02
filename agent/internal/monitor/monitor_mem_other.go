// ============================================================
// monitor_mem_other.go — 非 Windows 平台内存信息 fallback
//
// 在 Linux/macOS 上，内存获取走 monitor.go 中的 proc/meminfo
// 和 sysctl 路径，不需要 GlobalMemoryStatusEx。
// 此文件提供 getWindowsMemoryInfo 的空实现以通过编译。
// ============================================================

//go:build !windows

package monitor

// getWindowsMemoryInfo 非 Windows 平台的占位实现
//
// 此函数仅在 Windows 上被调用（通过 getTotalRAMWindows 和
// getUsedRAMWindows），但 Go 编译器要求所有平台都能看到函数定义。
// 非 Windows 平台返回空结构体。
func getWindowsMemoryInfo() windowsMemoryInfo {
	return windowsMemoryInfo{}
}
