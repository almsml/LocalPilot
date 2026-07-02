// ============================================================
// sandbox.go — 沙箱隔离层
//
// 在受限环境中执行外部命令，防止任务影响 Agent 或宿主机。
//
// 多层降级策略（按优先级）：
//   1. Docker 可用 → 创建容器执行（网络隔离、只读根文件系统）
//   2. Docker 不可用 → 进程级隔离（os/exec + setrlimit 资源限制）
//
// 为什么需要多层降级而不是只支持 Docker？
//   旧设备上跑 Docker 性能很差，Windows 上 Docker Desktop 不一定装了。
//   Docker 不可用时自动回退到基础进程隔离，让所有设备都能执行任务。
//
// Phase 2 策略：
//   先实现 ProcessSandbox（os/exec），所有设备都能用。
//   DockerSandbox 作为后续优化（需要安装 Docker SDK for Go）。
//
// 为什么 Sandbox 是接口而不是具体类型？
//   接口让 executor 不关心具体沙箱实现——将来加 DockerSandbox
//   不需要改 executor 的任何代码。
// ============================================================

package sandbox

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	pb "github.com/localpilot/proto/localpilot/v1"
)

// ============================================================
// ResourceLimits — 沙箱资源限制
//
// 为什么限制不是硬性的？
//   ProcessSandbox 通过 setrlimit 限制 CPU 时间和内存。
//   这些是操作系统级别的软/硬限制——进程超出限制时
//   会收到信号（SIGXCPU/SIGKILL），而不是精确的 cgroup 配额。
//   精确资源隔离需要 Docker 或 cgroups（Phase 2 后期）。
// ============================================================

// ResourceLimits 定义任务的资源上限
type ResourceLimits struct {
	// CPULimit 是 CPU 使用率上限（核心数，0 表示不限制）
	// 例如 1.0 = 1 个 CPU 核心
	CPULimit float64

	// MemoryLimitBytes 是内存上限（字节，0 表示不限制）
	MemoryLimitBytes uint64

	// TimeoutSec 是任务最长执行时间（秒，0 表示不限制）
	TimeoutSec uint32
}

// ============================================================
// RunResult — 命令执行结果
// ============================================================

// RunResult 包含命令执行的完整结果
type RunResult struct {
	ExitCode int    // 进程退出码（0 表示成功）
	Stdout   string // 标准输出内容（完整）
	Stderr   string // 标准错误内容（完整）
}

// ============================================================
// Sandbox — 沙箱接口
//
// 所有沙箱实现必须满足此接口。
// executor 只依赖此接口，不依赖具体实现。
// ============================================================

// Sandbox 定义了在隔离环境中执行命令的接口
type Sandbox interface {
	// Run 在沙箱中执行命令
	//
	// 参数：
	//   - ctx: 用于取消任务。ctx.Done() 时应终止进程。
	//   - command: 可执行文件路径
	//   - args: 命令参数列表
	//   - env: 环境变量（key=value）
	//   - limits: 资源限制
	//
	// 返回：
	//   - *RunResult: 命令执行结果（退出码、stdout、stderr）
	//   - error: 执行失败（非退出码错误，如命令找不到、沙箱初始化失败）
	Run(ctx context.Context, command string, args []string, env map[string]string, limits *ResourceLimits) (*RunResult, error)

	// Name 返回沙箱实现的名称（如 "process"、"docker"）
	Name() string
}

// ============================================================
// ProcessSandbox — 进程级隔离
//
// 用 Go 的 os/exec 包执行命令，通过 setrlimit 限制资源。
// 这是最基础的沙箱——没有网络隔离、没有文件系统隔离。
//
// 为什么 os/exec 足够作为 Phase 2 的沙箱？
//   LocalPilot 的初始场景是个人设备网格——用户在自己的设备上
//   运行自己的命令（视频转码、数据处理等），不存在恶意代码风险。
//   基础资源限制（防止内存泄漏拖垮 Agent）+ 超时控制
//   已经覆盖了 90% 的实际问题。
//
// setrlimit 在不同 OS 上的行为：
//   - Linux: setrlimit(RLIMIT_AS, ...) 限制虚拟内存
//   - macOS: 同上，但没有 RLIMIT_NPROC 限制
//   - Windows: Go 的 syscall.Setrlimit 在 Windows 上不支持。
//             用 Job Object API 或简单跳过（Phase 2 用超时兜底）。
// ============================================================

// ProcessSandbox 用操作系统进程实现沙箱
type ProcessSandbox struct{}

// Name 返回沙箱名称
func (p *ProcessSandbox) Name() string {
	return "process"
}

// Run 在子进程中执行命令
func (p *ProcessSandbox) Run(
	ctx context.Context,
	command string,
	args []string,
	env map[string]string,
	limits *ResourceLimits,
) (*RunResult, error) {
	// ---- 创建命令 ----
	// 为什么用 CommandContext 而不是 Command + 手动 kill？
	//   CommandContext 内部管理 context 生命周期——ctx.Done() 时
	//   自动调用 os.Process.Kill()。省去了手动管理 goroutine 的麻烦。
	var cmd *exec.Cmd
	if limits != nil && limits.TimeoutSec > 0 {
		// 如果指定了超时，创建带超时的 context
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(limits.TimeoutSec)*time.Second)
		defer cancel()
		cmd = exec.CommandContext(timeoutCtx, command, args...)
	} else {
		cmd = exec.CommandContext(ctx, command, args...)
	}

	// ---- 设置环境变量 ----
	// 为什么继承当前进程的环境变量？
	//   ffmpeg、python 等工具依赖 PATH 和其他系统环境变量。
	//   完全隔离的环境会让大多数命令无法执行。
	//   当前 setrlimit 方案的风险是任务可以读取 Agent 的环境变量，
	//   这在个人设备场景下是可接受的。
	cmd.Env = buildEnv(env)

	// ---- 设置资源限制 ----
	if limits != nil {
		applyLimits(cmd, limits)
	}

	// ---- 捕获 stdout/stderr ----
	// 为什么用 Pipe 而不是直接赋值 bytes.Buffer？
	//   Pipe 让 executor 可以边读边推送 LogChunk——
	//   不需要等命令执行完就能看到实时输出。
	//   这是终端日志实时流的关键。
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stdout 管道失败: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	// ---- 启动命令 ----
	slog.Debug("沙箱: 启动命令",
		"sandbox", p.Name(),
		"command", command,
		"args", args,
	)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动命令失败: %w", err)
	}

	// ---- 读取输出 ----
	// 并发读取 stdout 和 stderr——避免管道缓冲区满导致命令阻塞。
	// 为什么不用 io.Copy？
	//   io.Copy 是阻塞的，需要 goroutine 来并发读取。
	var stdoutBuf, stderrBuf []byte
	var readErr error
	var mu sync.Mutex

	// 并发读取 stdout
	go func() {
		data, err := io.ReadAll(stdoutPipe)
		mu.Lock()
		stdoutBuf = data
		if readErr == nil && err != nil {
			readErr = err
		}
		mu.Unlock()
	}()

	// 并发读取 stderr
	go func() {
		data, err := io.ReadAll(stderrPipe)
		mu.Lock()
		stderrBuf = data
		if readErr == nil && err != nil {
			readErr = err
		}
		mu.Unlock()
	}()

	// ---- 等待命令完成 ----
	// Wait 会等待 stdout/stderr 的读取 goroutine 完成
	// （因为 Pipe 在读取方关闭后才会关闭）
	waitErr := cmd.Wait()

	mu.Lock()
	defer mu.Unlock()

	if readErr != nil {
		return nil, fmt.Errorf("读取命令输出失败: %w", readErr)
	}

	// ---- 构建结果 ----
	exitCode := 0
	if waitErr != nil {
		// 尝试提取退出码
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("等待命令完成失败: %w", waitErr)
		}
	}

	slog.Debug("沙箱: 命令执行完成",
		"sandbox", p.Name(),
		"command", command,
		"exit_code", exitCode,
		"stdout_len", len(stdoutBuf),
		"stderr_len", len(stderrBuf),
	)

	return &RunResult{
		ExitCode: exitCode,
		Stdout:   string(stdoutBuf),
		Stderr:   string(stderrBuf),
	}, nil
}

// ============================================================
// 辅助函数
// ============================================================

// buildEnv 构建环境变量列表（key=value 格式）
//
// 为什么继承 os.Environ() 而不是清空？
//   PATH、HOME、TEMP 等系统变量对大多数命令是必需的。
//   完全清空环境会导致 echo、ffmpeg 等基础命令都找不到。
//   任务特定的变量（如 INPUT_FILE）通过参数传入，会覆盖继承的值。
func buildEnv(extra map[string]string) []string {
	base := make(map[string]string)

	// 继承当前进程的环境变量
	for _, e := range os.Environ() {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				base[e[:i]] = e[i+1:]
				break
			}
		}
	}

	// 覆盖/追加用户指定的环境变量
	for k, v := range extra {
		base[k] = v
	}

	// 转为 key=value 切片
	result := make([]string, 0, len(base))
	for k, v := range base {
		result = append(result, k+"="+v)
	}
	return result
}

// applyLimits 对命令应用资源限制
//
// setrlimit 在 Windows 上不支持（Go 的 syscall.Setrlimit 在 Windows 返回 ENOTSUP）。
// Windows 上跳过资源限制，依靠超时（context timeout）兜底。
func applyLimits(cmd *exec.Cmd, limits *ResourceLimits) {
	if runtime.GOOS == "windows" {
		// Windows 不支持 setrlimit，跳过
		// Phase 2 后期可以考虑用 Windows Job Object API 设置资源限制
		return
	}

	// TODO(Phase 2 后期): Linux/macOS 通过 setrlimit(RLIMIT_AS) 限制内存、
	// setrlimit(RLIMIT_CPU) 限制 CPU 时间。
	// Go 的 os/exec 不直接暴露 setrlimit——子进程 fork 后在 SysProcAttr
	// 中配置。当前先跳过资源限制，用 context timeout 兜底。
	_ = limits
}

// ============================================================
// Sandbox 工厂函数
//
// 为什么需要工厂函数而不是让调用方自己选择沙箱？
//   调用方（executor）不应该知道沙箱的具体实现。
//   工厂函数负责检测可用运行时，返回最优的沙箱实例。
//   将来加 DockerSandbox 时只需要在这里加检测逻辑。
// ============================================================

// DetectSandbox 检测可用的沙箱运行时，返回最优的
//
// 检测顺序：
//   1. Docker → 如果 Docker socket 存在，使用 DockerSandbox（Phase 2 后期）
//   2. 否则 → 使用 ProcessSandbox（总是可用）
//
// Phase 2 当前实现：只返回 ProcessSandbox。
func DetectSandbox() Sandbox {
	// TODO(Phase 2 后期): 检测 Docker 可用性
	// 检测 /var/run/docker.sock（Linux/macOS）或 \\.\pipe\docker_engine（Windows）
	// 如果存在，尝试 ping Docker daemon，成功后返回 DockerSandbox

	slog.Info("沙箱: 使用进程级隔离（ProcessSandbox）")
	return &ProcessSandbox{}
}

// ConvertProtoLimits 将 proto 的 ResourceLimits 转换为沙箱的 ResourceLimits
func ConvertProtoLimits(pbLimits *pb.ResourceLimits) *ResourceLimits {
	if pbLimits == nil {
		return nil
	}
	return &ResourceLimits{
		CPULimit:         float64(pbLimits.GetCpuLimit()),
		MemoryLimitBytes: pbLimits.GetMemLimitBytes(),
		TimeoutSec:       pbLimits.GetTimeoutSec(),
	}
}
