// ============================================================
// device.go — 设备注册表（内存缓存 + SQLite 持久化）
//
// 为什么需要内存 + SQLite 双层存储？
//   内存缓存：API 查询设备列表是高频操作（Dashboard 每 2 秒刷新一次），
//            每次查 SQLite 太慢。内存缓存用 RWMutex 保护，读操作几乎零开销。
//   SQLite：  持久化——Controller 重启后能恢复之前注册的设备列表。
//             但 volatile 指标（cpu_percent / used_ram / state）只存内存，
//             重启后所有设备从 OFFLINE 开始，等待 Agent 重连恢复。
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
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动，无需 CGO
)

// DeviceState 设备健康状态
type DeviceState string

const (
	StateOnline    DeviceState = "ONLINE"    // 心跳正常
	StateUnhealthy DeviceState = "UNHEALTHY" // 15 秒无心跳，可能网络波动
	StateOffline   DeviceState = "OFFLINE"   // 30 秒无心跳，确认离线
)

// ============================================================
// Device — 设备数据模型
//
// 为什么 JSON 标签用 snake_case 而不是 camelCase？
//   Dashboard 的 TypeScript 接口期望 snake_case 字段名。
//   Go 的默认 JSON 序列化使用字段原名（PascalCase），
//   不加标签的话前后端字段名对不上。
//
// 为什么 State 也加 json 标签？
//   gin 的 c.JSON() 使用 encoding/json 序列化，
//   没有 json 标签的字段会用字段原名。为了前后端契约一致，
//   所有对外暴露的字段都显式声明 json 标签。
// ============================================================

// Device 表示一台已注册的计算设备
type Device struct {
	ID                string      `json:"id"`                  // Controller 分配的 UUID
	Hostname          string      `json:"hostname"`            // 设备主机名
	OS                string      `json:"os"`                  // 操作系统
	Arch              string      `json:"arch"`                // CPU 架构
	CPUCores          uint32      `json:"cpu_cores"`           // 逻辑 CPU 核心数
	TotalRAMBytes     uint64      `json:"total_ram_bytes"`     // 总内存（字节）
	GPUInfo           string      `json:"gpu_info"`            // GPU 信息
	SupportedRuntimes []string    `json:"supported_runtimes"`  // 支持的运行时 ["docker", "process"]
	AgentAddress      string      `json:"agent_address"`       // Agent gRPC 地址（IP:Port）
	State             DeviceState `json:"state"`               // 当前健康状态
	CPUPercent        float64     `json:"cpu_percent"`         // 当前 CPU 使用率 (0.0-1.0)
	UsedRAMBytes      uint64      `json:"used_ram_bytes"`      // 当前已用内存（字节）
	CPUTemperature    float64     `json:"cpu_temperature"`     // CPU 温度（摄氏度）
	RunningTaskCount  uint32      `json:"running_task_count"`  // 当前正在执行的任务数
	LastHeartbeat     time.Time   `json:"last_heartbeat"`      // 最后一次心跳时间
	RegisteredAt      time.Time   `json:"registered_at"`       // 注册时间
}

// DeviceRegistry 设备注册表——Controller 的核心数据结构
//
// 线程安全：所有公开方法内部都有适当的锁保护。
type DeviceRegistry struct {
	mu      sync.RWMutex        // 保护 devices map 的并发读写
	devices map[string]*Device   // device_id → Device（内存缓存）
	store   *SQLiteStore         // SQLite 持久化
}

// NewDeviceRegistry 创建一个新的设备注册表
func NewDeviceRegistry(store *SQLiteStore) *DeviceRegistry {
	return &DeviceRegistry{
		devices: make(map[string]*Device),
		store:   store,
	}
}

// ============================================================
// Register 注册一台新设备
//
// 使用 UUID 生成唯一设备 ID。如果同一台设备（相同 hostname +
// agent_address）之前注册过（Controller 重启恢复场景），
// 复用旧的 device_id 以避免重复。
//
// 为什么用 UUID v4 而不是自增 ID？
//   自增 ID 依赖 SQLite 的有序性，分布式场景下不同 Controller
//   实例可能生成冲突的 ID。UUID v4 是全局唯一的标准方案。
// ============================================================

// Register 注册一台新设备
func (r *DeviceRegistry) Register(device *Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// ---- 检测重注册（Controller 重启后同一设备重新连接） ----
	// 同一 hostname + agent_address 的设备视为同一台物理设备。
	// 为什么不用 MAC 地址？
	//   Docker 容器的 MAC 是虚拟的，虚拟机可能克隆 MAC，
	//   hostname + IP:Port 组合在局域网内足够唯一。
	for _, existing := range r.devices {
		if existing.Hostname == device.Hostname &&
			existing.AgentAddress == device.AgentAddress {
			// 重注册：复用旧 UUID，更新设备信息
			device.ID = existing.ID
			device.RegisteredAt = existing.RegisteredAt // 保留首次注册时间
			goto finalize
		}
	}

	// ---- 首次注册：生成 UUID ----
	device.ID = uuid.New().String()
	device.RegisteredAt = time.Now()

finalize:
	device.State = StateOnline
	device.LastHeartbeat = time.Now()

	if device.RegisteredAt.IsZero() {
		device.RegisteredAt = time.Now()
	}

	// 存入内存
	r.devices[device.ID] = device

	// 持久化到 SQLite
	if err := r.store.SaveDevice(device); err != nil {
		return fmt.Errorf("持久化设备失败 (id=%s): %w", device.ID, err)
	}

	slog.Info("设备已注册",
		"device_id", device.ID,
		"hostname", device.Hostname,
		"agent_address", device.AgentAddress,
	)

	return nil
}

// UpdateHeartbeat 更新设备心跳（Agent 每 5 秒调用）
//
// 这是整个系统中最频繁的写操作。只更新内存中的运行时指标，
// 不写 SQLite——volatile 数据不需要持久化。
func (r *DeviceRegistry) UpdateHeartbeat(
	deviceID string,
	cpuPercent float64,
	usedRAM uint64,
	temperature float64,
	taskCount uint32,
) (*Device, error) {
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

// Deregister 从注册表中主动移除设备（Agent 主动注销）
func (r *DeviceRegistry) Deregister(deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.devices[deviceID]; !exists {
		return ErrDeviceNotFound
	}

	delete(r.devices, deviceID)
	return r.store.DeleteDevice(deviceID)
}

// RestoreDevices 从 SQLite 恢复设备到内存注册表（Controller 重启时使用）
//
// 为什么恢复时所有设备初始状态设为 OFFLINE？
//   Controller 重启意味着之前的心跳序列全部失效。
//   Agent 会在几秒内重新注册（通过 gRPC Register），
//   注册时状态会自动更新为 ONLINE。恢复时设 OFFLINE
//   是最保守也最正确的初始状态。
func (r *DeviceRegistry) RestoreDevices(devices []*Device) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, d := range devices {
		d.State = StateOffline
		d.CPUPercent = 0
		d.UsedRAMBytes = 0
		d.CPUTemperature = 0
		d.RunningTaskCount = 0
		r.devices[d.ID] = d
	}

	slog.Info("从数据库恢复了设备列表",
		"count", len(devices),
		"note", "所有设备初始状态为 OFFLINE，等待 Agent 重连")
}

// ============================================================
// SQLiteStore — SQLite 持久化层
//
// 为什么 SQLite 连接数设为 1？
//   SQLite 在 WAL 模式下支持一写多读，但写入是串行的。
//   SetMaxOpenConns(1) 避免了多个 goroutine 竞争写入
//   导致的 "database is locked" 错误。
//
// 为什么不持久化 volatile 字段（cpu_percent、state 等）？
//   这些字段每 5 秒更新一次。如果每次心跳都写 SQLite，
//   会产生大量写 IO。而且重启后这些数据已经过时——
//   设备状态从 OFFLINE 开始等心跳恢复才是有意义的数据。
// ============================================================

// SQLiteStore SQLite 数据库封装
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore 打开 SQLite 数据库并初始化表结构
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	// modernc.org/sqlite 是纯 Go 实现的 SQLite，无需 CGO
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 数据库失败: %w", err)
	}

	// 连接池配置：SQLite 只支持单写入者
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// 预热连接
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("SQLite 连接测试失败: %w", err)
	}

	// ---- 配置 PRAGMA ----
	// WAL 模式：写入不阻塞读取，适合 Dashboard 高频查询场景
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("启用 WAL 模式失败: %w", err)
	}
	// busy_timeout: 写入锁被占用时等待 5 秒而不是立即报错
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置 busy_timeout 失败: %w", err)
	}
	// synchronous=NORMAL: 在 WAL 模式下安全且性能更高
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("设置 synchronous 失败: %w", err)
	}
	// foreign_keys: 引用完整性检查（后续 jobs 表会用到）
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("启用外键失败: %w", err)
	}

	// ---- 建表 ----
	// 为什么用 IF NOT EXISTS？
	//   Controller 重启后表结构不变，IF NOT EXISTS 保证幂等。
	//   如果未来要加字段，用 ALTER TABLE 或版本化迁移脚本。
	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteStore{db: db}, nil
}

// createTables 执行建表语句
//
// devices 表只存储静态属性——设备能力（CPU 核数、内存大小等）
// 和连接信息（Agent 地址）。运行时指标（CPU 使用率、状态等）
// 不持久化，因为这些数据在重启后已经过时。
func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS devices (
		id                 TEXT PRIMARY KEY,
		hostname           TEXT NOT NULL,
		os                 TEXT NOT NULL DEFAULT '',
		arch               TEXT NOT NULL DEFAULT '',
		cpu_cores          INTEGER NOT NULL DEFAULT 0,
		total_ram_bytes    INTEGER NOT NULL DEFAULT 0,
		gpu_info           TEXT NOT NULL DEFAULT '',
		supported_runtimes TEXT NOT NULL DEFAULT '',
		agent_address      TEXT NOT NULL DEFAULT '',
		registered_at      INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_devices_hostname
		ON devices(hostname);

	CREATE INDEX IF NOT EXISTS idx_devices_agent_address
		ON devices(agent_address);
	`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("创建设备表失败: %w", err)
	}

	return nil
}

// DB 返回底层 *sql.DB（供 job store 等复用同一个数据库连接）
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// Close 关闭数据库连接
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ============================================================
// SaveDevice 持久化设备到 SQLite
//
// 使用 INSERT OR REPLACE：新设备插入，已有设备更新。
// 为什么只存静态字段？
//   设备能力（CPU 核数、总内存等）在设备生命周期内不变，
//   只在注册时写一次。运行时指标每次心跳都变，不写数据库。
// ============================================================

// SaveDevice 保存或更新设备记录
func (s *SQLiteStore) SaveDevice(device *Device) error {
	// 将 []string 序列化为逗号分隔字符串存储
	// SQLite 没有数组类型，逗号分隔是轻量级方案。
	// 为什么不用 JSON 字段？
	//   逗号分隔在这里够用——supported_runtimes 通常只有 ["docker", "process"]，
	//   用 JSON 反而增加序列化开销和查询复杂度。
	runtimes := ""
	for i, r := range device.SupportedRuntimes {
		if i > 0 {
			runtimes += ","
		}
		runtimes += r
	}

	query := `
	INSERT OR REPLACE INTO devices
		(id, hostname, os, arch, cpu_cores, total_ram_bytes,
		 gpu_info, supported_runtimes, agent_address, registered_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		device.ID,
		device.Hostname,
		device.OS,
		device.Arch,
		device.CPUCores,
		int64(device.TotalRAMBytes),
		device.GPUInfo,
		runtimes,
		device.AgentAddress,
		device.RegisteredAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("保存设备失败 (id=%s): %w", device.ID, err)
	}

	return nil
}

// DeleteDevice 从 SQLite 中删除设备
//
// 幂等操作——删除不存在的设备不算错误。
// 为什么幂等？
//   如果 Agent 发送了两次 Deregister（网络重试），
//   第二次应该静默成功而不是报错。
func (s *SQLiteStore) DeleteDevice(deviceID string) error {
	_, err := s.db.Exec("DELETE FROM devices WHERE id = ?", deviceID)
	if err != nil {
		return fmt.Errorf("删除设备失败 (id=%s): %w", deviceID, err)
	}
	return nil
}

// ============================================================
// LoadDevices 从 SQLite 恢复设备列表（Controller 重启时调用）
//
// 只恢复静态属性。运行时指标（state, cpu_percent 等）
// 在 RestoreDevices() 中初始化为 OFFLINE。
// ============================================================

// LoadDevices 从 SQLite 加载所有持久化的设备
func (s *SQLiteStore) LoadDevices() ([]*Device, error) {
	query := `
	SELECT id, hostname, os, arch, cpu_cores, total_ram_bytes,
	       gpu_info, supported_runtimes, agent_address, registered_at
	FROM devices
	ORDER BY registered_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("查询设备列表失败: %w", err)
	}
	defer rows.Close()

	var devices []*Device
	for rows.Next() {
		d := &Device{}

		var totalRAM int64
		var registeredAt int64
		var runtimesStr string

		err := rows.Scan(
			&d.ID,
			&d.Hostname,
			&d.OS,
			&d.Arch,
			&d.CPUCores,
			&totalRAM,
			&d.GPUInfo,
			&runtimesStr,
			&d.AgentAddress,
			&registeredAt,
		)
		if err != nil {
			return nil, fmt.Errorf("扫描设备行失败: %w", err)
		}

		d.TotalRAMBytes = uint64(totalRAM)
		d.RegisteredAt = time.Unix(registeredAt, 0)

		// 反序列化 supported_runtimes（逗号分隔 → []string）
		if runtimesStr != "" {
			d.SupportedRuntimes = splitRuntimes(runtimesStr)
		} else {
			d.SupportedRuntimes = []string{}
		}

		devices = append(devices, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历设备行时出错: %w", err)
	}

	return devices, nil
}

// splitRuntimes 将逗号分隔的字符串拆分为 []string
func splitRuntimes(s string) []string {
	if s == "" {
		return []string{}
	}
	result := make([]string, 0)
	current := ""
	for _, ch := range s {
		if ch == ',' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// ============================================================
// 错误定义
// ============================================================

var (
	ErrDeviceNotFound = &RegistryError{"设备未找到"}
)

// RegistryError 注册表业务错误
type RegistryError struct {
	Message string
}

func (e *RegistryError) Error() string {
	return e.Message
}
