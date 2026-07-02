// ============================================================
// device.go — 设备注册表（内存缓存 + SQLite 持久化）
//
// 为什么需要内存 + SQLite 双层存储？
//   内存缓存：API 查询设备列表是高频操作（Dashboard 每 2 秒刷新一次），
//            每次查 SQLite 太慢。内存缓存用 RWMutex 保护，读操作几乎零开销。
//   SQLite：  持久化——Controller 重启后能恢复之前注册的设备列表。
//             但设备状态（ONLINE/OFFLINE）只存在内存里，
//             重启后所有设备重新注册。
//
// 为什么用 sync.RWMutex 而不是 channel？
//   读操作（GetDevice、ListDevices）远多于写操作（心跳更新状态）。
//   RWMutex 允许多个读操作并发执行，写操作独占。
//   channel 模式在这里是过度设计——一个简单的锁就够了。
// ============================================================

package registry

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动，无需 CGO
)

// DeviceState 设备健康状态
type DeviceState string

const (
	StateOnline    DeviceState = "ONLINE"    // 心跳正常
	StateUnhealthy DeviceState = "UNHEALTHY" // 15 秒无心跳，可能网络波动
	StateOffline   DeviceState = "OFFLINE"   // 30 秒无心跳，确认离线
)

// Device 表示一台已注册的计算设备
type Device struct {
	ID                string            // Controller 分配的唯一 ID（UUID）
	Hostname          string            // 设备主机名
	OS                string            // 操作系统
	Arch              string            // CPU 架构
	CPUCores          uint32            // 逻辑 CPU 核心数
	TotalRAMBytes     uint64            // 总内存（字节）
	GPUInfo           string            // GPU 信息
	SupportedRuntimes []string          // 支持的运行时 ["docker", "process"]
	AgentAddress      string            // Agent gRPC 地址（IP:Port）
	State             DeviceState       // 当前健康状态
	CPUPercent        float64           // 当前 CPU 使用率
	UsedRAMBytes      uint64            // 当前已用内存
	CPUTemperature    float64           // CPU 温度（摄氏度）
	RunningTaskCount  uint32            // 当前正在执行的任务数
	LastHeartbeat     time.Time         // 最后一次心跳时间
	RegisteredAt      time.Time         // 注册时间
}

// DeviceRegistry 设备注册表——Controller 的核心数据结构
//
// 线程安全：所有公开方法内部都有适当的锁保护。
type DeviceRegistry struct {
	mu      sync.RWMutex           // 保护 devices map 的并发读写
	devices map[string]*Device      // device_id → Device（内存缓存）
	store   *SQLiteStore            // SQLite 持久化
}

// NewDeviceRegistry 创建一个新的设备注册表
func NewDeviceRegistry(store *SQLiteStore) *DeviceRegistry {
	return &DeviceRegistry{
		devices: make(map[string]*Device),
		store:   store,
	}
}

// Register 注册一台新设备
//
// Controller 分配 device_id，将设备信息存入内存和 SQLite。
// 如果设备之前注册过（相同 hostname + agent_address），复用旧的 device_id。
func (r *DeviceRegistry) Register(device *Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 生成 device_id（简化版——用 hostname 作为 ID，生产环境应用 UUID）
	// TODO: 用 github.com/google/uuid 生成真正的 UUID
	device.ID = device.Hostname + "-" + device.AgentAddress
	device.State = StateOnline
	device.LastHeartbeat = time.Now()
	device.RegisteredAt = time.Now()

	// 存入内存
	r.devices[device.ID] = device

	// 持久化到 SQLite
	return r.store.SaveDevice(device)
}

// UpdateHeartbeat 更新设备心跳（Agent 每 5 秒调用）
//
// 这是整个系统中最频繁的写操作，所以只用写锁保护，
// 并且更新 SQLite 时只更新心跳时间戳（不更新全量字段）。
func (r *DeviceRegistry) UpdateHeartbeat(deviceID string, cpuPercent float64, usedRAM uint64, temperature float64, taskCount uint32) (*Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	device, exists := r.devices[deviceID]
	if !exists {
		return nil, ErrDeviceNotFound
	}

	// 更新内存中的实时指标
	device.State = StateOnline
	device.CPUPercent = cpuPercent
	device.UsedRAMBytes = usedRAM
	device.CPUTemperature = temperature
	device.RunningTaskCount = taskCount
	device.LastHeartbeat = time.Now()

	return device, nil
}

// GetDevice 根据 device_id 获取设备信息
func (r *DeviceRegistry) GetDevice(deviceID string) (*Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	device, exists := r.devices[deviceID]
	if !exists {
		return nil, ErrDeviceNotFound
	}
	return device, nil
}

// ListDevices 返回所有已注册设备的列表
func (r *DeviceRegistry) ListDevices() []*Device {
	r.mu.RLock()
	defer r.mu.RUnlock()

	devices := make([]*Device, 0, len(r.devices))
	for _, d := range r.devices {
		devices = append(devices, d)
	}
	return devices
}

// Deregister 从注册表中移除设备
func (r *DeviceRegistry) Deregister(deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.devices, deviceID)
	return r.store.DeleteDevice(deviceID)
}

// ============================================================
// SQLiteStore — SQLite 持久化层
//
// 当前 Phase 0 简化为接口占位。
// Phase 1 会实现完整的建表 + CRUD。
// ============================================================

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	// modernc.org/sqlite 是纯 Go 实现的 SQLite，无需 CGO
	// DSN 格式简单：直接传文件路径
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// 连接池配置：SQLite 只支持单写入者
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// 预热连接
	if err := db.Ping(); err != nil {
		return nil, err
	}

	// 启用 WAL 模式（必须手动执行 PRAGMA，modernc 的 DSN 不支持 query params）
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("启用 WAL 模式失败: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("设置 busy_timeout 失败: %w", err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return nil, fmt.Errorf("设置 synchronous 失败: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("启用外键失败: %w", err)
	}

	// Phase 1: 执行建表语句
	// CREATE TABLE IF NOT EXISTS devices (...)
	// CREATE TABLE IF NOT EXISTS jobs (...)

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) SaveDevice(device *Device) error {
	// Phase 1: 实现 INSERT OR REPLACE
	return nil
}

func (s *SQLiteStore) DeleteDevice(deviceID string) error {
	// Phase 1: 实现 DELETE
	return nil
}

// ============================================================
// 错误定义
// ============================================================

var (
	ErrDeviceNotFound = &RegistryError{"设备未找到"}
)

type RegistryError struct {
	Message string
}

func (e *RegistryError) Error() string {
	return e.Message
}
