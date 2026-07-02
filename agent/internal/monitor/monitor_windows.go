// ============================================================
// monitor_windows.go — Windows 平台内存信息获取
//
// 使用 golang.org/x/sys/windows 的 GlobalMemoryStatusEx API。
// 通过构建标签 //go:build windows 确保只在 Windows 上编译。
//
// 为什么单独一个文件？
//   Go 的条件编译（build constraints）要求平台特定的代码
//   放在独立文件中。这样在 Linux/macOS 上编译时不会引用
//   Windows API，避免链接错误。
// ============================================================

//go:build windows

package monitor

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// memoryStatusEx Windows MEMORYSTATUSEX 结构体
//
// 与 kernel32.dll 的 GlobalMemoryStatusEx 函数配合使用。
// 字段对应 WIN32 API 中的 MEMORYSTATUSEX：
//
//	dwLength     - 结构体大小
//	dwMemoryLoad - 内存使用百分比（0-100）
//	ullTotalPhys - 物理内存总量
//	ullAvailPhys - 可用物理内存
//	ullTotalPageFile - 页面文件总量
//	ullAvailPageFile - 可用页面文件
//	ullTotalVirtual  - 虚拟内存总量
//	ullAvailVirtual  - 可用虚拟内存
type memoryStatusEx struct {
	dwLength        uint32
	dwMemoryLoad    uint32
	ullTotalPhys    uint64
	ullAvailPhys    uint64
	ullTotalPf      uint64
	ullAvailPf      uint64
	ullTotalVi      uint64
	ullAvailVi      uint64
	ullAvailExtVi   uint64
}

// getWindowsMemoryInfo 通过 GlobalMemoryStatusEx 获取 Windows 内存信息
//
// 实现细节：
//   1. 初始化 MEMORYSTATUSEX 结构体，设置 dwLength
//   2. 调用 kernel32.GlobalMemoryStatusEx
//   3. 提取 ullTotalPhys（总物理内存）和 ullAvailPhys（可用物理内存）
//
// 为什么用 GlobalMemoryStatusEx 而不是 GlobalMemoryStatus？
//   GlobalMemoryStatusEx 支持超过 4GB 的内存（使用 64 位字段），
//   GlobalMemoryStatus 在 >4GB 时会返回错误值（上限截断）。
func getWindowsMemoryInfo() windowsMemoryInfo {
	var msx memoryStatusEx
	msx.dwLength = uint32(unsafe.Sizeof(msx))

	// 加载 kernel32.dll 并调用 GlobalMemoryStatusEx
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&msx)))
	if ret == 0 {
		// 调用失败，返回空信息
		return windowsMemoryInfo{}
	}

	return windowsMemoryInfo{
		totalPhys: msx.ullTotalPhys,
		availPhys: msx.ullAvailPhys,
	}
}
