// ============================================================
// manager.go — Job 生命周期管理器
//
// 编排任务的完整生命周期：
//   SubmitJob → 持久化 → 入队 → worker 取任务 → 选设备 →
//   gRPC Execute Agent → 收集日志 → 更新状态
//
// 为什么用独立的 worker goroutine 而不是每个 job 一个 goroutine？
//   worker 模式保证同一时间只处理一个任务——简单且安全。
//   多任务并发需要调度器（Phase 3）来决定"哪个任务分配给哪个设备"。
//   Phase 2 先用单 worker + 第一个 ONLINE 设备的简单策略。
// ============================================================

package job

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	pb "github.com/localpilot/proto/localpilot/v1"
	"github.com/localpilot/controller/internal/registry"
	"github.com/localpilot/controller/internal/scheduler"
	"github.com/localpilot/controller/internal/transport"
)

// ============================================================
// Manager — 任务生命周期管理器
// ============================================================

// Manager 管理所有任务的提交、调度和执行
type Manager struct {
	store       *Store                   // SQLite 持久化
	queue       *Queue                   // 任务队列
	registry    *registry.DeviceRegistry // 设备注册表
	scheduler   *scheduler.Scheduler     // 多维打分调度器
	mu          sync.RWMutex
	runningJobs map[string]*Job          // 内存中的任务（含日志）
}

// NewManager 创建任务管理器
func NewManager(store *Store, queue *Queue, reg *registry.DeviceRegistry, sch *scheduler.Scheduler) *Manager {
	return &Manager{
		store:       store,
		queue:       queue,
		registry:    reg,
		scheduler:   sch,
		runningJobs: make(map[string]*Job),
	}
}

// StartWorker 启动后台 worker goroutine
//
// 从队列中取任务，分配到在线设备，通过 gRPC 下发执行。
// 为什么在独立的 goroutine 中运行？
//   任务执行可能持续几分钟甚至几小时。
//   worker 独立运行，不阻塞 HTTP API 和其他 Controller 功能。
func (m *Manager) StartWorker(ctx context.Context) {
	go func() {
		slog.Info("Job worker 已启动")
		for {
			select {
			case <-ctx.Done():
				slog.Info("Job worker 收到退出信号")
				return
			default:
				// 从队列取任务（阻塞等待）
				job := m.queue.Dequeue()

				// 执行任务
				m.executeJob(ctx, job)
			}
		}
	}()
}

// ============================================================
// SubmitJob 提交一个新任务
//
// 用户从 Dashboard 提交命令 → 创建 Job → 持久化 → 入队。
//
// 为什么先持久化再入队？
//   持久化失败说明存储有问题——此时不应该执行任务。
//   先持久化保证任务不会在队列中丢失。
// ============================================================

// SubmitJob 提交新任务
func (m *Manager) SubmitJob(name string, command string, args []string, env map[string]string) (*Job, error) {
	// 创建任务对象
	// 为什么 env 要初始化为空 map 而不是 nil？
	//   JSON 序列化 nil map 输出 "null"，空 map 输出 "{}"。
	//   Dashboard 的 TypeScript 类型期望 Record<string, string>，
	//   null 会导致运行时错误。
	if env == nil {
		env = make(map[string]string)
	}
	if args == nil {
		args = []string{}
	}

	job := &Job{
		ID:        uuid.New().String(),
		Name:      name,
		Command:   command,
		Args:      args,
		Env:       env,
		Status:    StateQueued,
		Logs:      make([]LogLine, 0),
		CreatedAt: time.Now(),
	}

	// 持久化
	if err := m.store.SaveJob(job); err != nil {
		return nil, fmt.Errorf("持久化任务失败: %w", err)
	}

	// 跟踪到内存
	m.mu.Lock()
	m.runningJobs[job.ID] = job
	m.mu.Unlock()

	// 入队
	m.queue.Enqueue(job)

	slog.Info("任务已提交",
		"job_id", job.ID,
		"name", job.Name,
		"command", job.Command,
	)

	return job, nil
}

// GetJob 查询任务状态（含日志）
//
// 先查内存（运行中的任务有日志），再查 SQLite。
func (m *Manager) GetJob(jobID string) (*Job, error) {
	m.mu.RLock()
	if j, ok := m.runningJobs[jobID]; ok {
		m.mu.RUnlock()
		return j, nil
	}
	m.mu.RUnlock()
	return m.store.GetJob(jobID)
}

// ListJobs 列出所有任务
func (m *Manager) ListJobs() ([]*Job, error) {
	jobs, err := m.store.ListJobs()
	if err != nil {
		return nil, err
	}

	// 合并在内存中的任务日志
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i, j := range jobs {
		if running, ok := m.runningJobs[j.ID]; ok {
			jobs[i] = running
		}
	}
	return jobs, nil
}

// ============================================================
// executeJob 执行单个任务
//
// 执行流程：
//   1. 找一个在线的设备
//   2. 标记任务为 ASSIGNED → RUNNING
//   3. 通过 gRPC 连接 Agent 的 TaskExecutionService
//   4. 调用 Execute，接收 server-streaming 的 LogChunk
//   5. 将 LogChunk 存入 Job.Logs（供 Dashboard 查询）
//   6. 根据 stream 关闭方式判断 COMPLETED / FAILED
//
// Phase 2 的简单设备选择：
//   选第一个 ONLINE 设备。如果没找到在线设备，跳过此任务
//   （任务保留在队列中，下次 worker 检查时重新尝试）。
// ============================================================

// executeJob 执行任务
func (m *Manager) executeJob(parentCtx context.Context, job *Job) {
	slog.Info("开始执行任务", "job_id", job.ID, "command", job.Command)

	// ---- 1. 多维调度器选设备 ----
	// 调度器对每台在线设备按 CPU/内存/架构/延迟/GPU 打分，选最优。
	device := m.scheduler.SelectDevice()
	if device == nil {
		slog.Warn("没有在线设备，任务稍后重试", "job_id", job.ID)
		// 重新入队（简单重试策略——Phase 3 会改进）
		m.queue.Enqueue(job)
		return
	}

	// ---- 2. 更新任务状态 ----
	job.Status = StateAssigned
	job.DeviceID = device.ID
	m.store.UpdateStatus(job.ID, StateAssigned)
	m.store.UpdateDevice(job.ID, device.ID)

	// ---- 3. 连接 Agent ----
	ctx, cancel := context.WithTimeout(parentCtx, 10*time.Minute)
	defer cancel()

	client, err := transport.ConnectToAgent(ctx, device.AgentAddress)
	if err != nil {
		slog.Error("连接 Agent 失败", "job_id", job.ID, "device", device.AgentAddress, "error", err)
		job.Status = StateFailed
		m.store.UpdateStatus(job.ID, StateFailed)
		return
	}
	defer client.Close()

	// ---- 4. 下发任务 ----
	// 将 Job 的 env map 转为 proto 需要的格式
	pbEnv := make(map[string]string)
	if job.Env != nil {
		for k, v := range job.Env {
			pbEnv[k] = v
		}
	}

	req := &pb.ExecuteRequest{
		TaskId:  job.ID,
		Command: job.Command,
		Args:    job.Args,
		Env:     pbEnv,
	}

	stream, err := client.Execute(ctx, req)
	if err != nil {
		slog.Error("下发任务失败", "job_id", job.ID, "error", err)
		job.Status = StateFailed
		m.store.UpdateStatus(job.ID, StateFailed)
		return
	}

	// ---- 5. 标记为 RUNNING ----
	job.Status = StateRunning
	m.store.UpdateStatus(job.ID, StateRunning)

	// ---- 6. 接收实时日志 ----
	var lastExitCode int
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			// 流正常结束——Agent 执行完成
			break
		}
		if err != nil {
			slog.Error("接收日志流失败", "job_id", job.ID, "error", err)
			job.Status = StateFailed
			m.store.UpdateStatus(job.ID, StateFailed)
			return
		}

		// 记录日志行
		streamType := "stdout"
		if chunk.GetStreamType() == pb.StreamType_STREAM_TYPE_STDERR {
			streamType = "stderr"
		}

		job.Logs = append(job.Logs, LogLine{
			StreamType: streamType,
			Data:       string(chunk.GetData()),
			Timestamp:  time.Now(),
		})

		// 检查是否是完成日志
		// Agent 在 Execute 结束时发送 "[DONE]" 或 "[ERROR]" 标记
		data := string(chunk.GetData())
		if len(data) > 7 && data[:7] == "[ERROR]" {
			job.Status = StateFailed
		}
	}

	// ---- 7. 更新最终状态 ----
	if job.Status != StateFailed {
		job.Status = StateCompleted
		job.ExitCode = lastExitCode
	}
	m.store.UpdateStatus(job.ID, job.Status)

	slog.Info("任务执行完成",
		"job_id", job.ID,
		"status", job.Status,
		"exit_code", job.ExitCode,
		"log_lines", len(job.Logs),
	)
}
// ============================================================
// MigrateTasksFromDevice 迁移离线设备上的任务（Phase 4）
//
// 设备 OFFLINE 时自动将该设备上 ASSIGNED/RUNNING 的任务
// 重置为 QUEUED 并重新入队，由调度器分配到其他在线设备执行。
// ============================================================

// MigrateTasksFromDevice 迁移指定设备上的所有活跃任务
func (m *Manager) MigrateTasksFromDevice(deviceID string) {
	m.mu.RLock()
	var toMigrate []*Job
	for _, j := range m.runningJobs {
		if j.DeviceID == deviceID && (j.Status == StateAssigned || j.Status == StateRunning) {
			toMigrate = append(toMigrate, j)
		}
	}
	m.mu.RUnlock()

	if len(toMigrate) == 0 {
		return
	}

	slog.Warn("迁移离线设备任务", "device_id", deviceID, "count", len(toMigrate))

	for _, j := range toMigrate {
		j.Status = StateQueued
		j.DeviceID = ""
		m.store.UpdateStatus(j.ID, StateQueued)
		m.queue.Enqueue(j)
		slog.Info("任务重新入队", "job_id", j.ID, "command", j.Command)
	}
}
