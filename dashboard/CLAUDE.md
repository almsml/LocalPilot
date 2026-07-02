# dashboard/ — React 前端仪表板

## 一句话

Dashboard 是用户看到的 Web 界面：设备状态实时刷新、任务提交表单、终端日志查看器、集群拓扑图。

## 模块地图

```
src/
├── main.tsx                  — 入口（React Query Provider + Toaster）
├── App.tsx                   — 根组件（设备列表 → 将来扩展为多 Tab 布局）
├── api/
│   └── client.ts             — API 客户端（fetch 封装）
├── types/
│   └── index.ts              — TypeScript 类型（对应 proto message）
├── hooks/
│   ├── useDevices.ts         — 设备列表 React Query Hook（自动刷新）
│   └── useWebSocket.ts       — WebSocket 连接（Phase 2）
└── components/
    ├── DeviceGrid.tsx         — 设备卡片网格
    ├── DeviceCard.tsx         — 单个设备卡片（状态指示、CPU/内存/温度）
    ├── DeviceDetail.tsx       — 设备详情面板（Phase 1）
    ├── JobSubmitter.tsx       — 任务提交表单（Phase 2）
    ├── JobTimeline.tsx        — 任务时间线（Phase 2）
    ├── TerminalLog.tsx        — 实时终端日志（Phase 2）
    ├── ClusterTopology.tsx    — 集群拓扑图 React Flow（Phase 5）
    └── MetricsDashboard.tsx   — 集群指标仪表板（Phase 5）
```

## 关键设计决策

1. **为什么用 React Query 而不是 useEffect + fetch？**
   React Query 自动管理缓存、后台刷新、loading/error 状态。设备列表需要定期刷新——React Query 的 staleTime + refetchInterval 比手写 setInterval 干净得多。

2. **为什么用 fetch 而不是 axios？**
   浏览器内置 fetch 已经足够好用。对于 LocalPilot 的 API 调用量（QPS < 10），不需要 axios 的拦截器、取消请求等高级功能。

3. **为什么 Vite 代理 API 请求？**
   Dashboard (5173) 和 Controller (8080) 端口不同，浏览器有跨域限制。Vite 的 proxy 配置让开发环境请求自动转发，不需要在 Controller 侧加 CORS（虽然我们加了 CORS 作为备选）。

## 运行

```bash
cd dashboard
npm install
npm run dev
# 打开 http://localhost:5173
```

## 当前状态：Phase 0

- ✅ 项目骨架（Vite + React + TypeScript）
- ✅ 设备列表 API 客户端
- ✅ 设备卡片 UI（状态颜色、系统信息展示）
- ⏳ 设备详情面板（Phase 1）
- ⏳ 数据自动刷新（Phase 1）
- ⏳ 任务提交 + 终端日志（Phase 2）
