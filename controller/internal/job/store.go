// ============================================================
// store.go — Job SQLite 持久化层
//
// 将任务的状态和定义持久化到 SQLite，保证 Controller
// 重启后任务信息不丢失。
//
// 为什么不持久化日志（stdout/stderr）？
//   日志数据量大（ffmpeg 转码可能产生几 MB 输出），
//   频繁写 SQLite 会严重影响性能。
//   日志只在内存中保存，Controller 重启后日志丢失可接受。
// ============================================================

package job

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ============================================================
// JobState — 任务状态
// ============================================================

// JobState 任务生命周期状态
type JobState string

const (
	StateQueued    JobState = "QUEUED"    // 等待调度执行
	StateAssigned  JobState = "ASSIGNED"  // 已分配到设备
	StateRunning   JobState = "RUNNING"   // 正在执行
	StateCompleted JobState = "COMPLETED" // 执行成功
	StateFailed    JobState = "FAILED"    // 执行失败
	StateCancelled JobState = "CANCELLED" // 用户取消
)

// ============================================================
// Job — 任务模型
// ============================================================

// Job 表示一个计算任务
type Job struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Command    string            `json:"command"`
	Args       []string          `json:"args"`
	Env        map[string]string `json:"env"`
	Status     JobState          `json:"status"`
	DeviceID   string            `json:"device_id"`
	ExitCode   int               `json:"exit_code"`
	Logs       []LogLine         `json:"logs"` // 仅内存，不持久化到 SQLite
	CreatedAt  time.Time         `json:"created_at"`
}

// LogLine 单行任务日志
type LogLine struct {
	StreamType string    `json:"stream_type"` // "stdout" 或 "stderr"
	Data       string    `json:"data"`        // 日志内容
	Timestamp  time.Time `json:"timestamp"`   // 记录时间
}

// ============================================================
// Store — Job SQLite 持久化
// ============================================================

// Store 管理 Job 的 SQLite 持久化
type Store struct {
	db *sql.DB
}

// NewStore 打开数据库并建表
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.createTable(); err != nil {
		return nil, err
	}
	return s, nil
}

// createTable 创建 jobs 表
//
// 为什么 args 和 env 用 JSON 存储？
//   SQLite 不支持数组和 map 类型，JSON TEXT 列是最简单的方案。
func (s *Store) createTable() error {
	schema := `
	CREATE TABLE IF NOT EXISTS jobs (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL DEFAULT '',
		command     TEXT NOT NULL,
		args        TEXT NOT NULL DEFAULT '[]',
		env         TEXT NOT NULL DEFAULT '{}',
		status      TEXT NOT NULL DEFAULT 'QUEUED',
		device_id   TEXT NOT NULL DEFAULT '',
		exit_code   INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	`
	_, err := s.db.Exec(schema)
	return err
}

// SaveJob 保存或更新任务
func (s *Store) SaveJob(job *Job) error {
	argsJSON, _ := json.Marshal(job.Args)
	envJSON, _ := json.Marshal(job.Env)

	query := `
	INSERT OR REPLACE INTO jobs
		(id, name, command, args, env, status, device_id, exit_code, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		job.ID, job.Name, job.Command,
		string(argsJSON), string(envJSON),
		string(job.Status), job.DeviceID, job.ExitCode,
		job.CreatedAt.Unix(),
	)
	return err
}

// GetJob 按 ID 查询
func (s *Store) GetJob(jobID string) (*Job, error) {
	var j Job
	var argsJSON, envJSON string
	var createdAt int64

	err := s.db.QueryRow(
		`SELECT id, name, command, args, env, status, device_id, exit_code, created_at
		 FROM jobs WHERE id = ?`, jobID,
	).Scan(&j.ID, &j.Name, &j.Command, &argsJSON, &envJSON,
		(*string)(&j.Status), &j.DeviceID, &j.ExitCode, &createdAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("任务未找到: %s", jobID)
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(argsJSON), &j.Args)
	json.Unmarshal([]byte(envJSON), &j.Env)
	j.CreatedAt = time.Unix(createdAt, 0)

	return &j, nil
}

// ListJobs 列出所有任务
func (s *Store) ListJobs() ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT id, name, command, args, env, status, device_id, exit_code, created_at
		 FROM jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		var j Job
		var argsJSON, envJSON string
		var createdAt int64

		if err := rows.Scan(&j.ID, &j.Name, &j.Command, &argsJSON, &envJSON,
			(*string)(&j.Status), &j.DeviceID, &j.ExitCode, &createdAt); err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(argsJSON), &j.Args)
		json.Unmarshal([]byte(envJSON), &j.Env)
		j.CreatedAt = time.Unix(createdAt, 0)

		jobs = append(jobs, &j)
	}
	return jobs, rows.Err()
}

// UpdateStatus 更新任务状态
func (s *Store) UpdateStatus(jobID string, status JobState) error {
	_, err := s.db.Exec("UPDATE jobs SET status = ? WHERE id = ?", string(status), jobID)
	return err
}

// UpdateDevice 更新分配的设备
func (s *Store) UpdateDevice(jobID string, deviceID string) error {
	_, err := s.db.Exec("UPDATE jobs SET device_id = ? WHERE id = ?", deviceID, jobID)
	return err
}
