# Quantix Connector 工业级 Go 重构计划（7x24 + 毫秒级响应）

## Summary
- 目标升级为工业级：在“实用兼容”前提下完成 Go 后端 + HTML/JS 前端全量替换，并满足 7x24 稳定运行。
- 强制SLA：
  - 采集响应：端到端 `P95 <= 20ms`（设备返回到 API/WS 可见）。
  - 可用性：`99.9%`（单机强化架构）。
  - 容量：约 `10 台设备 @ 10Hz` 首发目标。
- 范围不变：业务接口 + 调试接口 + OpenAPI/docs + 全页面能力；一次性切换；空库启动；Windows + macOS 同级支持。

## Key Changes (Implementation)
1. 工业级运行架构（优先实现）
- 事件驱动 + 高频轮询混合调度：每设备独立 runtime worker、无阻塞 I/O、优先级队列。
- 关键路径预算分解（驱动读、解析、状态写入、WS广播）并做超时与熔断控制。
- 单机强化机制：进程看门狗、runtime 自愈重启、驱动断线退避重连、背压与限流、队列水位保护。
- 数据可靠性：本地 WAL/持久缓冲（仅关键状态与诊断日志），异常恢复后快速回放。

2. 契约与兼容实现（冻结不变）
- 完整保留接口与模型语义：`/health`、`/ws`、`/api/devices*`、`/api/protocols*`、`/api/serial-debug*`、`/api/printers/*`、`/api/scanners/*`、`/api/boards/*`。
- 保留鉴权双通道（`X-API-Key` + `api_key`），保留 WS 心跳 `{"type":"ping"}`，保留 4 类事件结构。
- 协议执行器严格对齐：`setup/poll/manual/event`、parse 规则、`allow_write` 安全门。

3. 工业实时优化专项
- 驱动层：串口/TCP/MQTT/Modbus 统一超时模型与错误码归一化；Windows/macOS 差异封装。
- 调度层：固定周期漂移校正、慢设备隔离（不拖垮全局）、扫码去重窗口保持兼容。
- 广播层：WS 发布与采集线程解耦，防止前端慢连接影响采集时延。
- 可观测性：内建指标（延迟分位数、重连次数、丢包/超时、队列深度、CPU/内存）与健康探针分级。

4. 前端重构（功能同等）
- 维持 6 大页面能力与字段语义不变，实时大屏采用 WS 主通道 + 10 秒兜底。
- 增加工业诊断视图（延迟/状态/重连计数）用于现场运维，不改变原有业务接口。

5. 切换与发布
- 先通过全量契约 + 性能 + 稳定性验收，再一次性切换。
- 保留旧 Python 服务回退脚本（同端口快速切回）。

## Public APIs / Interfaces / Types
- 完整锁定并保持不变：
  - 设备：`/api/devices`、`/api/devices/{id}`、`/api/devices/by-code/{device_code}` + enable/disable/execute。
  - 协议：`/api/protocols`、`/import`、`/{id}`、`/{id}/export`、`/{id}/test`、`/{id}/test-step`。
  - 分类：`/api/printers/{id}/print`、`/api/scanners/{id}/last`、`/api/boards/{id}/status`。
  - 串口调试：`/api/serial-debug/ports|status|open|close|send|read|logs`。
  - 实时：`WS /ws?api_key=...`（`weight_update/print_event/scan_event/board_event/ping`）。
- 文档端点保持：`/openapi.json`、`/docs`。

## Test Plan (Industrial Acceptance)
1. 契约回归
- 全接口 golden tests：成功/401/400/404/409、字段与状态码对齐旧系统。
- WS：鉴权失败关闭码、事件结构一致、心跳行为一致。

2. 实时性能验收
- 在 `10台@10Hz` 工况下压测：
  - 端到端延迟 `P95 <= 20ms`。
  - 连续运行 24h 延迟漂移与抖动在阈值内。
- 慢设备/断线/抖动注入测试：验证隔离与自愈有效。

3. 7x24稳定性验收
- 72h 连续稳定性测试（首轮），记录重连、自恢复、内存增长、CPU 峰值。
- 故障演练：串口拔插、网络抖动、MQTT broker 短时不可用、进程重启恢复。
- 可用性统计达到 99.9% 目标口径。

4. 平台验收
- Windows + macOS 双平台各完成 Serial/TCP/MQTT/Modbus 至少 1 套实机联调。
- 双平台启动/部署/回退脚本验收通过。

## Assumptions
- 兼容级别为“实用兼容”；错误文案不做逐字强制。
- 前端为功能等价，不做像素级复刻。
- 空库启动，不迁移历史数据。
- 上线为一次性全量替换。
- 首发平台为 Windows 与 macOS 同级支持。
- 工业级验收以 `P95<=20ms`、`99.9%可用性`、`10台@10Hz` 为硬门槛。
