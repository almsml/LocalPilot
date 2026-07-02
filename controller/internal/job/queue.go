// ============================================================
// queue.go — Job 任务队列（FIFO）
//
// 基于 channel 的先进先出队列。新任务从队尾入队，
// worker goroutine 从队头取任务执行。
//
// 为什么用 channel 而不是 slice？
//   channel 天然支持阻塞等待——worker 调用 Dequeue() 时
//   如果队列为空，goroutine 自动挂起，不消耗 CPU。
//   slice 需要轮询或条件变量，更复杂且容易出错。
//
// 为什么 Phase 2 用简单 FIFO 而不是优先级队列？
//   优先级调度需要多维打分引擎（Phase 3）。
//   先跑通端到端链路，再优化调度策略。
// ============================================================

package job

import (
	"sync"
)

// Queue 基于 channel 的 FIFO 任务队列
type Queue struct {
	mu   sync.Mutex
	jobs []*Job           // 排队中的任务
	ch   chan *Job        // 通知 channel
}

// NewQueue 创建任务队列
func NewQueue() *Queue {
	return &Queue{
		jobs: make([]*Job, 0),
		ch:   make(chan *Job, 100), // 缓冲 100 个任务
	}
}

// Enqueue 将任务加入队列
//
// 为什么先加到 slice 再往 channel 发？
//   保证 Dequeue() 返回的顺序严格按入队顺序（FIFO）。
//   如果直接往 channel 发，多个 goroutine 同时 Enqueue
//   时顺序不能保证。
func (q *Queue) Enqueue(job *Job) {
	q.mu.Lock()
	q.jobs = append(q.jobs, job)
	q.mu.Unlock()

	// 通知等待的 worker
	select {
	case q.ch <- job:
	default:
		// channel 满了说明 worker 处理不过来，但任务已经在 slice 中
		// worker 会从 slice 中取出
	}
}

// Dequeue 从队列头部取出一个任务
//
// 如果队列为空，阻塞等待直到有新任务入队。
// 这是 channel 的核心优势——不消耗 CPU 的等待。
func (q *Queue) Dequeue() *Job {
	for {
		q.mu.Lock()
		if len(q.jobs) > 0 {
			job := q.jobs[0]
			q.jobs = q.jobs[1:]
			q.mu.Unlock()
			return job
		}
		q.mu.Unlock()

		// 队列空，等待新任务入队
		<-q.ch
	}
}

// Len 返回当前队列长度（用于监控）
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs)
}
