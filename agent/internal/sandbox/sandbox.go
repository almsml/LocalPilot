// ============================================================
// sandbox.go — 沙箱隔离层
//
// Phase 2 实现。多层降级策略：
//   1. Docker 可用 → 创建容器执行
//   2. Docker 不可用 → Linux cgroups + namespaces 进程隔离
//   3. macOS → sandbox-exec
//   4. 都不行 → 基础进程隔离（至少限制资源）
//
// 为什么需要多层降级而不是只支持 Docker？
//   树莓派上跑 Docker 性能很差，Windows 上 Docker Desktop 不一定装了。
//   给用户选择的空间，而不是强依赖一个运行时。
//
// 与 Rust 版本 (agent/src/sandbox.rs) 对应。
// ============================================================

package sandbox

// TODO(Phase 2): 实现沙箱抽象层。
//
// 设计要点：
//   1. 定义 Sandbox 接口：Run(ctx, command, args, limits) → (<stdout>, <stderr>, error)
//   2. 实现 DockerSandbox / ProcessSandbox / CGroupsSandbox
//   3. 自动检测可用运行时并选择最优方案
//   4. 资源限制：CPU shares, memory limit, disk quota
//
// Go 库选型：
//   - Docker SDK: github.com/docker/docker/client
//   - cgroups: 通过 /sys/fs/cgroup 文件系统操作
//   - macOS sandbox: 通过 sandbox-exec 命令行工具
