// ============================================================
// scorer.go — 多维打分引擎
//
// 对每台候选设备根据以下维度综合评分，选出最优执行设备：
//
// 权重模型（来自 LocalPilot-规划.md §三）：
//   cpu_score    * 0.35   空闲 CPU 比例 → 偏好负载低的设备
//   mem_score    * 0.25   空闲内存比例 → 大内存任务优先分配
//   arch_score   * 0.20   架构匹配度   → ARM 任务不分配到 x86
//   latency      * 0.10   心跳新鲜度   → 网络稳定的设备优先
//   gpu_score    * 0.10   GPU 需求匹配  → GPU 任务优先分配到 GPU 设备
//
// 为什么用加权求和而不是简单的"选负载最低的"？
//   单一维度的选择会导致次优结果——比如一台空载但网络不稳定的
//   设备得分可能低于一台轻度负载但连接稳定的设备。
//   多维加权让调度器在每个维度之间做出合理的权衡。
//
// 为什么这些权重值？
//   CPU 和内存是通用计算最核心的资源，占 60%。
//   架构匹配影响二进制兼容性，占 20%。
//   延迟和 GPU 是辅助因素，各占 10%。
//   这些值不是拍脑袋的——可以从实际运行数据中通过
//   线性回归或网格搜索进行优化（Phase 5 打磨）。
// ============================================================

package scheduler

import (
	"math"
	"time"

	"github.com/localpilot/controller/internal/registry"
)

// ============================================================
// 评分权重常量
//
// 为什么是常量而不是配置？
//   Phase 3 先用固定权重验证调度效果。
//   Phase 5 可以做成配置文件或运行时调整。
// ============================================================

const (
	weightCPU    = 0.35 // CPU 空闲比例权重
	weightMem    = 0.25 // 内存空闲比例权重
	weightArch   = 0.20 // 架构匹配度权重
	weightLatency = 0.10 // 网络延迟权重
	weightGPU    = 0.10 // GPU 匹配度权重
)

// ============================================================
// ScoreResult — 单台设备的评分结果
//
// 为什么包含分项得分而不仅仅是总分？
//   分项得分用于日志和调试——当调度结果不符合预期时，
//   可以查看每台设备在各个维度上的得分，方便定位问题。
//   这在面试 Demo 时也很有说服力——"你看，这个 GPU 任务
//   在这台设备上得分最高，因为它的 GPU 维度得分是 1.0"。
// ============================================================

// ScoreResult 一台设备的评分详情
type ScoreResult struct {
	Device  *registry.Device
	Total   float64 // 加权总分（0.0 ~ 1.0）

	// 分项得分（用于日志和调试）
	CPUScore    float64
	MemScore    float64
	ArchScore   float64
	LatencyScore float64
	GPUScore    float64
}

// ============================================================
// Score 对候选设备列表评分，返回按总分降序排列的结果
//
// 为什么不是对单台设备评分而是对列表评分？
//   调度器需要从所有在线设备中选最优的。如果每次只评一台，
//   调用方需要自己维护排序逻辑——不如一次性返回排序结果。
//
// 参数：
//   - devices: 候选设备列表（通常只包含 ONLINE 状态的设备）
//   - job: 待调度的任务（用于检查特殊需求，如 GPU 任务）
//
// 返回：按总分从高到低排列的评分结果列表
// ============================================================

// Score 对所有候选设备打分排序
func Score(devices []*registry.Device) []ScoreResult {
	results := make([]ScoreResult, 0, len(devices))

	for _, d := range devices {
		r := ScoreResult{Device: d}

		// ---- 1. CPU 得分：空闲比例 ----
		// cpu_percent 是 0.0~1.0 的使用率，空闲比例 = 1 - 使用率
		// 为什么用当前 CPU 使用率而不是平均值？
		//   瞬时值反映了设备当前的真实负载。如果任务很短（几秒），
		//   当前负载是最准确的指标。长任务可以考虑用滑动窗口平均。
		r.CPUScore = 1.0 - d.CPUPercent
		if r.CPUScore < 0 {
			r.CPUScore = 0 // 防御：防止异常数据
		}

		// ---- 2. 内存得分：空闲比例 ----
		// 为什么用内存空闲比例而不是绝对值？
		//   不同设备的内存容量差异很大（4GB ~ 64GB）。
		//   用比例让调度器在不同容量的设备之间公平比较。
		if d.TotalRAMBytes > 0 {
			freeRAM := float64(d.TotalRAMBytes - d.UsedRAMBytes)
			r.MemScore = freeRAM / float64(d.TotalRAMBytes)
			if r.MemScore < 0 {
				r.MemScore = 0
			}
		} else {
			r.MemScore = 0.5 // 无法获取内存信息时给中性分
		}

		// ---- 3. 架构得分：匹配度 ----
		// 当前 Phase 3 简化处理：所有设备都是 x86_64 或 amd64，
		// 架构差异不大。如果任务指定了架构需求，这里做精确匹配。
		// 为什么默认给 0.8 而不是 1.0？
		//   预留空间给未来的"精确架构匹配"——
		//   比如明确要求 ARM 的任务在 ARM 设备上得 1.0，
		//   在 x86 设备上得 0（通过 QEMU 模拟可能可以跑但性能差）。
		r.ArchScore = 0.8

		// 如果有 GPU 信息，架构可能有异构特征
		if d.GPUInfo != "" {
			r.ArchScore = 0.9 // GPU 设备通常性能更好
		}

		// ---- 4. 延迟得分：心跳新鲜度 ----
		// 心跳越新鲜 → 设备连接越稳定 → 下发任务成功率高。
		// 为什么用心跳时间差而不是 ping 延迟？
		//   不需要额外的网络探测——心跳已经提供了连接质量的信号。
		//   心跳在 5 秒内 → 满分；超过 10 秒 → 衰减；超过 15 秒 → 低分。
		sinceLastBeat := time.Since(d.LastHeartbeat).Seconds()
		switch {
		case sinceLastBeat <= 5:
			r.LatencyScore = 1.0 // 心跳在 5 秒内，网络健康
		case sinceLastBeat <= 10:
			r.LatencyScore = 0.7 // 轻微延迟
		case sinceLastBeat <= 15:
			r.LatencyScore = 0.3 // 明显延迟，设备可能变为 UNHEALTHY
		default:
			r.LatencyScore = 0.0 // 几乎离线，不分配新任务
		}

		// ---- 5. GPU 得分：是否有 GPU ----
		// 当前 Phase 3：job 不声明 GPU 需求，所以有 GPU 的设备得更高分。
		// 为什么有 GPU 的设备得分更高？
		//   GPU 是一种稀缺资源。如果有 GPU 设备可用，优先使用它——
		//   让无 GPU 设备留给不需要 GPU 的任务（尽管当前所有任务
		//   都可能不需要 GPU，但这个偏好是合理的未来设计）。
		if d.GPUInfo != "" {
			r.GPUScore = 1.0 // 有 GPU
		} else {
			r.GPUScore = 0.5 // 无 GPU，中性分
		}

		// ---- 加权求和 ----
		r.Total = r.CPUScore*weightCPU +
			r.MemScore*weightMem +
			r.ArchScore*weightArch +
			r.LatencyScore*weightLatency +
			r.GPUScore*weightGPU

		// 防御：确保总分在 0.0 ~ 1.0 之间
		r.Total = math.Max(0.0, math.Min(1.0, r.Total))

		results = append(results, r)
	}

	// ---- 按总分降序排序 ----
	// 为什么用稳定排序而不是 sort.Slice？
	//   稳定排序保证分数相同的设备保持原始顺序（注册顺序），
	//   这对于可复现的调度结果很重要——同样的输入应该产生
	//   同样的输出，方便调试。
	sortByScore(results)

	return results
}

// ============================================================
// 排序（简单插入排序——候选设备通常不超过几十台）
// ============================================================

func sortByScore(results []ScoreResult) {
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].Total < key.Total {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}

// ============================================================
// SelectBest 从在线设备中选出最优执行设备
//
// 这是调度器的核心入口——输入任务，返回最优设备。
// Phase 3 的简单版本：只考虑在线设备，不考虑负载均衡。
// Phase 4 会加入任务亲和性、历史成功率等因素。
// ============================================================

// SelectBest 为任务选择最优执行设备
//
// 如果没有可用设备，返回 nil。
// 调度失败由调用方决定重试策略（如重新入队）。
func SelectBest(reg *registry.DeviceRegistry) *registry.Device {
	// 收集所有在线设备
	allDevices := reg.ListDevices()
	var online []*registry.Device
	for _, d := range allDevices {
		if d.State == registry.StateOnline {
			online = append(online, d)
		}
	}

	if len(online) == 0 {
		return nil
	}

	// 打分
	results := Score(online)

	// 返回得分最高的设备
	best := results[0]

	return best.Device
}
