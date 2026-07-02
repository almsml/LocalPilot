// ============================================================
// executor.go — 任务执行器
//
// Phase 2 实现。负责：
//   1. 接收 Task → 创建工作目录
//   2. 从 Controller 拉取输入文件
//   3. 在沙箱中执行命令并捕获 stdout/stderr
//   4. 实时推送日志（通过 gRPC streaming）
//   5. 上传结果文件 + 清理
//
// 与 Rust 版本 (agent/src/executor.rs) 对应。
// ============================================================

package executor

// TODO(Phase 2): 实现任务执行核心逻辑。
// 当前 Phase 0: 只验证 gRPC streaming 链路。
//
// 设计要点：
//   1. 每个任务一个 goroutine——Go 的并发模型天然适合
//   2. 使用 os/exec 包执行外部命令
//   3. 通过 io.Pipe 或 cmd.StdoutPipe 捕获实时输出
//   4. 将输出封装成 pb.LogChunk 通过 channel 发送给 gRPC stream
//   5. 监听 context.Done() 实现任务取消
