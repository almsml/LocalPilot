// ============================================================
// executor.go — 任务执行器
//
// 负责接收 Task 并在沙箱中执行，将 stdout/stderr 实时
// 封装为 LogChunk 推送给调用方（gRPC server-stream）。
//
// 为什么每个任务一个 goroutine？
//   多个任务可以并发执行——一台设备可能同时跑多个任务
//   （例如分片任务的不同分片）。Go 的 goroutine 轻量到
//   可以轻松支持数十个并发任务。
//
// 为什么用 channel 而不是直接写入 gRPC stream？
//   channel 解耦了命令执行和网络传输：
//   - 命令执行很快（写 stdout pipe → channel）
//   - gRPC 传输可能慢（网络波动、Controller 处理慢）
//   - buffer 防止慢消费者阻塞命令执行
// ============================================================

package executor

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	pb "github.com/localpilot/proto/localpilot/v1"
	"github.com/localpilot/agent/internal/sandbox"
)

// ============================================================
// Executor — 任务执行器
//
// 管理所有活跃任务的 goroutine。每个任务有一个唯一 ID，
// 通过 tasks map 可以查询和取消。
// ============================================================

// Executor 管理任务执行
type Executor struct {
	mu    sync.RWMutex
	tasks map[string]*runningTask // taskID → 正在运行的任务

	sandbox sandbox.Sandbox // 沙箱实现（ProcessSandbox 或 DockerSandbox）
}

// runningTask 表示一个正在运行的任务
type runningTask struct {
	cmd    *exec.Cmd     // 正在运行的命令
	cancel context.CancelFunc // 取消任务的函数
}

// New 创建一个新的任务执行器
//
// 参数：
//   - sb: 沙箱实现（通过 DetectSandbox() 获取）
func New(sb sandbox.Sandbox) *Executor {
	return &Executor{
		tasks:   make(map[string]*runningTask),
		sandbox: sb,
	}
}

// ============================================================
// ExecuteResult — 任务执行结果
// ============================================================

// ExecuteResult 包含任务执行的输出流
type ExecuteResult struct {
	// LogChunks 是从命令的 stdout/stderr 实时读取的日志块
	LogChunks <-chan *pb.LogChunk

	// Done 在任务完成时关闭（成功或失败）
	Done <-chan struct{}

	// Result 在 Done 关闭后可用（如果 err != nil 说明执行失败）
	Result *sandbox.RunResult
	Err    error
}

// ============================================================
// Execute 执行任务
//
// 这是 executor 的核心方法。它在 goroutine 中：
//   1. 创建工作目录
//   2. 拉取输入文件（Phase 2: 先跳过，假设文件已在本地）
//   3. 通过沙箱执行命令
//   4. 将 stdout/stderr 逐行封装为 LogChunk 推送到 channel
//
// 返回 ExecuteResult，调用方通过 LogChunks channel 接收实时日志。
//
// 为什么用逐行推送而不是整块推送？
//   逐行推送让 Dashboard 能看到命令的实时输出——就像在终端里
//   看到 ping 命令一行一行输出一样。整块推送只有等命令结束
//   才能看到结果。
// ============================================================

// Execute 在沙箱中执行任务并返回实时日志流
func (e *Executor) Execute(
	ctx context.Context,
	taskID string,
	command string,
	args []string,
	env map[string]string,
	limits *pb.ResourceLimits,
) *ExecuteResult {
	logChunks := make(chan *pb.LogChunk, 100) // 缓冲 100 条日志
	done := make(chan struct{})

	result := &ExecuteResult{
		LogChunks: logChunks,
		Done:      done,
	}

	go func() {
		defer close(done)
		defer close(logChunks)

		// ---- 1. 创建工作目录 ----
		// 每个任务有独立的工作目录，防止文件冲突。
		// 为什么用 taskID 作为目录名？
		//   taskID 是 UUID——全局唯一，不会有命名冲突。
		workDir := filepath.Join(os.TempDir(), "localpilot", taskID)
		if err := os.MkdirAll(workDir, 0755); err != nil {
			result.Err = fmt.Errorf("创建工作目录失败: %w", err)
			slog.Error("任务执行失败", "task_id", taskID, "error", result.Err)
			return
		}
		defer os.RemoveAll(workDir) // 执行完成后清理

		// ---- 2. 记录任务开始 ----
		slog.Info("任务开始执行",
			"task_id", taskID,
			"command", command,
			"args", args,
			"work_dir", workDir,
		)

		// ---- 3. 执行命令 ----
		sbLimits := sandbox.ConvertProtoLimits(limits)

		// 用带取消的 context 包装——Cancel() 可以通过 cancel 函数终止命令
		taskCtx, taskCancel := context.WithCancel(ctx)

		// 注册到 tasks map，以便 Cancel() 可以找到它
		e.registerTask(taskID, nil, taskCancel)

		// 执行命令并逐行读取输出
		runResult, runErr := e.runCommand(taskCtx, taskID, command, args, env, sbLimits, logChunks, workDir)

		// 从 tasks map 中移除
		e.unregisterTask(taskID)

		if runErr != nil {
			// 沙箱执行失败（非退出码错误——如命令找不到）
			result.Err = runErr
			slog.Error("任务执行失败", "task_id", taskID, "error", runErr)
			return
		}

		result.Result = runResult
		slog.Info("任务执行完成",
			"task_id", taskID,
			"exit_code", runResult.ExitCode,
		)
	}()

	return result
}

// ============================================================
// runCommand 在沙箱中执行命令并逐行读取输出
//
// 为什么用 bufio.Scanner 逐行读取？
//   终端日志的"实时感"来自逐行显示。如果一次读 4096 字节，
//   可能读到半行输出，Dashboard 显示会错位。
//   bufio.Scanner 按换行符分割，保证每个 LogChunk 是一行完整的输出。
// ============================================================

// platformCommand 将命令转换为平台可执行的形式
//
// 为什么需要这个转换？
//   Windows 上 echo/dir/type 等常用命令是 cmd.exe 的内置命令，
//   没有独立的 .exe 文件。需要包装为 cmd /c <command> 来执行。
//   Linux/macOS 上这些命令通常有独立二进制（/bin/echo 等），
//   但也有些是 shell builtin——这里统一处理跨平台差异。
func platformCommand(command string, args []string) (string, []string) {
	if runtime.GOOS == "windows" {
		// 如果命令包含路径分隔符（如 ./script.sh 或 C:\tools\ffmpeg.exe），
		// 说明是外部可执行文件，不需要包装。
		if !containsPathSep(command) {
			// 包装为: cmd /c <command> <args...>
			allArgs := append([]string{"/c", command}, args...)
			return "cmd", allArgs
		}
	}
	return command, args
}

// containsPathSep 检查字符串是否包含路径分隔符
func containsPathSep(s string) bool {
	for _, ch := range s {
		if ch == '/' || ch == '\\' {
			return true
		}
	}
	return false
}

// runCommand 执行命令并通过 channel 推送实时日志
func (e *Executor) runCommand(
	ctx context.Context,
	taskID string,
	command string,
	args []string,
	env map[string]string,
	limits *sandbox.ResourceLimits,
	logChunks chan<- *pb.LogChunk,
	workDir string,
) (*sandbox.RunResult, error) {
	// ---- 平台适配 ----
	command, args = platformCommand(command, args)

	// ---- 创建命令 ----
	var cmd *exec.Cmd
	if limits != nil && limits.TimeoutSec > 0 {
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, time.Duration(limits.TimeoutSec)*time.Second)
		defer timeoutCancel()
		cmd = exec.CommandContext(timeoutCtx, command, args...)
	} else {
		cmd = exec.CommandContext(ctx, command, args...)
	}

	cmd.Dir = workDir
	cmd.Env = buildEnvList(env)

	// ---- 捕获 stdout/stderr ----
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stdout 管道失败: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stderr 管道失败: %w", err)
	}

	// ---- 启动命令 ----
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动命令失败: %w", err)
	}

	// 注册正在运行的命令，以便 Cancel() 可以杀死它
	e.registerTask(taskID, cmd, nil)

	// ---- 逐行读取 stdout/stderr ----
	// 用 WaitGroup 确保两个 reader goroutine 都完成后再 Wait()
	var wg sync.WaitGroup
	wg.Add(2)

	var seqNum uint64
	var mu sync.Mutex

	// 读取 stdout
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		// 设置更大的缓冲区——某些命令（如 ffmpeg）单行输出可能很长
		scanner.Buffer(make([]byte, 64*1024), 256*1024)
		for scanner.Scan() {
			mu.Lock()
			seq := seqNum
			seqNum++
			mu.Unlock()

			select {
			case logChunks <- &pb.LogChunk{
				TaskId:    taskID,
				StreamType: pb.StreamType_STREAM_TYPE_STDOUT,
				Data:      []byte(scanner.Text() + "\n"),
				SeqNum:    int64(seq),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// 读取 stderr
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 64*1024), 256*1024)
		for scanner.Scan() {
			mu.Lock()
			seq := seqNum
			seqNum++
			mu.Unlock()

			select {
			case logChunks <- &pb.LogChunk{
				TaskId:    taskID,
				StreamType: pb.StreamType_STREAM_TYPE_STDERR,
				Data:      []byte(scanner.Text() + "\n"),
				SeqNum:    int64(seq),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// 等待 stdout/stderr 全部读取完成
	wg.Wait()

	// ---- 等待命令完成 ----
	waitErr := cmd.Wait()

	// ---- 构建结果 ----
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("等待命令完成失败: %w", waitErr)
		}
	}

	return &sandbox.RunResult{
		ExitCode: exitCode,
	}, nil
}

// ============================================================
// Cancel 取消一个正在执行的任务
//
// 先尝试优雅终止（SIGTERM → 给进程清理机会），
// 如果进程不响应则强制杀死（SIGKILL）。
//
// 为什么先 SIGTERM 再 SIGKILL？
//   SIGTERM 让进程有机会做清理工作（删除临时文件、flush 缓冲区）。
//   强制 kill 是最后手段——只用在进程不响应的时候。
// ============================================================

// Cancel 取消任务
func (e *Executor) Cancel(taskID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	task, exists := e.tasks[taskID]
	if !exists {
		return fmt.Errorf("任务未找到或已完成: %s", taskID)
	}

	// ---- 优先取消 context ----
	// cancel 会触发 context.Done()，命令的 CommandContext 会收到信号
	// 并自动 kill 进程。
	if task.cancel != nil {
		task.cancel()
		slog.Info("任务已取消（context）", "task_id", taskID)
	}

	// ---- 兜底：直接 kill 进程 ----
	// 如果 cancel 后进程还没退出（某些进程不响应 context 取消），
	// 直接 kill。这是双重保险。
	if task.cmd != nil && task.cmd.Process != nil {
		if err := task.cmd.Process.Kill(); err != nil {
			slog.Warn("强制终止任务进程失败", "task_id", taskID, "error", err)
		} else {
			slog.Info("任务进程已强制终止", "task_id", taskID)
		}
	}

	return nil
}

// ============================================================
// 内部方法
// ============================================================

// registerTask 注册一个运行中的任务
func (e *Executor) registerTask(taskID string, cmd *exec.Cmd, cancel context.CancelFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tasks[taskID] = &runningTask{
		cmd:    cmd,
		cancel: cancel,
	}
}

// unregisterTask 从任务表中移除
func (e *Executor) unregisterTask(taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.tasks, taskID)
}

// ============================================================
// 辅助函数
// ============================================================

// buildEnvList 构建环境变量列表
func buildEnvList(extra map[string]string) []string {
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

	// 覆盖用户指定的环境变量
	for k, v := range extra {
		base[k] = v
	}

	result := make([]string, 0, len(base))
	for k, v := range base {
		result = append(result, k+"="+v)
	}
	return result
}
