# Quantix Connector 常见设备模板清单

更新时间：2026-03-21

本文基于旧仓库计划文档 `/Users/n/Code/Quantix-Cnnector/docs/plans/2026-03-09-serial-printer-scanner-board-implementation-plan.md`，并结合当前 Go 项目种子模板（`internal/store/seed.go`）整理。

旧项目文档模板全集（文档内联版）见：

- [legacy-device-templates-full.md](/Users/n/Code/Quantix-Cnnector-go/docs/legacy-device-templates-full.md)

## 1. 标准 Modbus TCP 称重模板

- 模板名：`Std-Modbus-Scale`
- 协议类型：`modbus_tcp`
- 设备分类建议：`weight`
- 典型用途：工业称重仪表轮询采集
- 关键动作：
  - `modbus.read_input_registers`
- 关键变量：
  - `slave_id`（默认 `1`）
  - `address`（默认 `0`）
- 输出：
  - `weight`
  - `unit`（`kg`）

推荐连接参数示例（与 `modbus_tcp_test_server.py` 对齐）：

```json
{
  "host": "127.0.0.1",
  "port": 1502
}
```

模板变量示例：

```json
{
  "slave_id": 1,
  "address": 0
}
```

## 2. MQTT 推送称重模板

- 模板名：`MQTT-Weight-Sensor`
- 协议类型：`mqtt`
- 设备分类建议：`weight`
- 典型用途：设备主动上报重量，不做轮询
- 关键动作：
  - `mqtt.subscribe`
  - `mqtt.on_message`（事件触发）
- 关键变量：
  - `topic`（默认 `sensor/weight`）
- 输出：
  - `weight`
  - `unit`（`kg`）

推荐连接参数示例：

```json
{
  "host": "127.0.0.1",
  "port": 1883
}
```

## 3. TSC 串口打印模板

- 模板名：`TSC-Serial-Print`
- 协议类型：`serial`
- 设备分类建议：`printer_tsc`
- 典型用途：串口标签打印，支持 ACK 校验
- 关键动作：
  - `serial.send`
  - `serial.receive`
- 关键变量：
  - `print_data`
  - `ack_timeout`
  - `job_id`
- 输出：
  - `print_ack`
  - `job_id`

推荐连接参数示例（Windows）：

```json
{
  "port": "COM3",
  "baudrate": 9600,
  "bytesize": 8,
  "parity": "N",
  "stopbits": 1,
  "timeout": 1.0
}
```

推荐连接参数示例（macOS）：

```json
{
  "port": "/dev/tty.usbserial-1410",
  "baudrate": 9600,
  "bytesize": 8,
  "parity": "N",
  "stopbits": 1,
  "timeout": 1.0
}
```

## 4. TSC TCP 打印模板

- 模板名：`TSC-TCP-Print`
- 协议类型：`tcp`
- 设备分类建议：`printer_tsc`
- 典型用途：网络打印机（以太网）
- 关键动作：
  - `tcp.send`（支持 `wait_ack` 和 `ack_pattern`）
- 关键变量：
  - `print_data`
  - `ack_timeout`
  - `job_id`
- 输出：
  - `print_ack`
  - `job_id`

推荐连接参数示例：

```json
{
  "host": "192.168.1.100",
  "port": 9100
}
```

## 5. 串口扫码枪行模式模板

- 模板名：`Serial-Scanner-LineMode`
- 协议类型：`serial`
- 设备分类建议：`scanner`
- 典型用途：条码扫描枪行结束符模式
- 关键动作：
  - `serial.receive`
- 关键变量：
  - `delimiter`
  - `symbology`
  - `dedupe_window_ms`
- 输出：
  - `barcode`
  - `symbology`

推荐连接参数示例（Windows）：

```json
{
  "port": "COM5",
  "baudrate": 9600,
  "bytesize": 8,
  "parity": "N",
  "stopbits": 1,
  "timeout": 0.3
}
```

## 6. 串口看板轮询模板

- 模板名：`Serial-Board-Polling`
- 协议类型：`serial`
- 设备分类建议：`serial_board`
- 典型用途：看板状态/数值轮询
- 关键动作：
  - `serial.send`
  - `serial.receive`
- 关键变量：
  - `poll_cmd`
  - `delimiter`
  - `unit`
- 输出：
  - `board_value`
  - `board_status`
  - `alarm`
  - `unit`

推荐连接参数示例（macOS）：

```json
{
  "port": "/dev/tty.usbserial-1420",
  "baudrate": 9600,
  "bytesize": 8,
  "parity": "N",
  "stopbits": 1,
  "timeout": 0.35
}
```

## 7. 使用建议（工业现场）

- 称重采集：
  - `poll_interval` 建议 `0.05 ~ 0.2` 秒（50~200ms）
  - 网络链路与 PLC/仪表响应必须实测
- 扫码去重窗口：
  - 建议 `300 ~ 800ms`
- 打印 ACK 超时：
  - 建议 `300 ~ 1500ms`，按设备型号调参
- 串口参数：
  - 必须与设备固件一致（波特率/校验位/停止位）
- 首次联调：
  - 先用“串口调试”或“设备管理-测试连接”确认链路，再开业务轮询

## 8. 梅特勒托利多模板

适用说明：

- 适配常见 MT-SICS/ASCII 串口称重设备
- 支持轮询读取、手动去皮、手动清零
- 默认命令：`SI`（读重量）、`T`（去皮）、`Z`（清零）

推荐连接参数（Windows）：

```json
{
  "port": "COM3",
  "baudrate": 9600,
  "bytesize": 8,
  "parity": "N",
  "stopbits": 1,
  "timeout": 1.0
}
```

推荐连接参数（macOS）：

```json
{
  "port": "/dev/tty.usbserial-1410",
  "baudrate": 9600,
  "bytesize": 8,
  "parity": "N",
  "stopbits": 1,
  "timeout": 1.0
}
```

模板全文见：

- [legacy-device-templates-full.md](/Users/n/Code/Quantix-Cnnector-go/docs/legacy-device-templates-full.md)（第 5 节）

### MT-SICS TCP 低延迟模板

适用说明：

- 适配梅特勒托利多 MT-SICS 协议台秤/天平的 TCP Server 模式
- 推荐用于低延迟轮询场景
- 默认读取命令使用 `SIU`，优先获取当前单位的即时重量，不等待稳定

推荐连接参数：

```json
{
  "host": "192.168.3.22",
  "port": 9761
}
```

推荐模板变量：

```json
{
  "read_command": "SIU\r\n",
  "tare_command": "T\r\n",
  "zero_command": "Z\r\n",
  "receive_size": 64,
  "timeout_ms": 80
}
```

推荐设备参数：

```json
{
  "device_category": "weight",
  "poll_interval": 0.05
}
```

建议：

- 追求极低延迟时，`poll_interval` 建议从 `0.05` 秒开始压测
- 若现场网络稳定，可进一步试 `0.03` 秒
- 若串口调试助手偶发乱码，通常是编码显示或拆包问题；TCP 模板按 ASCII 文本解析，稳定性更好

## 9. 奥豪斯 Navigator 模板

适用说明：

- 适配奥豪斯 Navigator（NV/NVL/NVT）USB 虚拟串口
- 支持轮询读取、打印、去皮、清零、切换单位/模式
- 默认命令：`P`、`SP`、`IP`、`T`、`Z`、`U`、`M`

推荐连接参数（Windows）：

```json
{
  "port": "COM3",
  "baudrate": 9600,
  "bytesize": 8,
  "parity": "N",
  "stopbits": 1,
  "timeout": 1.2
}
```

模板全文见：

- [legacy-device-templates-full.md](/Users/n/Code/Quantix-Cnnector-go/docs/legacy-device-templates-full.md)（第 6 节）

## 10. Modbus TCP 调试专用模板

适用说明：

- 用于联调阶段快速验证读写寄存器/线圈
- 覆盖 `poll` 连续采集与 `manual` 手工读写
- 适配设备管理中的手动执行接口

模板全文见：

- [legacy-device-templates-full.md](/Users/n/Code/Quantix-Cnnector-go/docs/legacy-device-templates-full.md)（第 14 节）

## 11. Modbus TCP（/1000 + 小数位可配置）模板

适用说明：

- 适配 `tools/modbus_tcp_test_server.py` 的 32 位有符号重量寄存器（2 个输入寄存器）
- 重量换算使用“除以 1000”（原始值 `1000` 显示为 `1`）
- 显示小数位由模板变量 `decimals` 控制（前端按 `output.decimals` 渲染）
- 重量单位由模板变量 `unit` 控制（前端按 `output.unit` 渲染）

模板全文见：

- [legacy-device-templates-full.md](/Users/n/Code/Quantix-Cnnector-go/docs/legacy-device-templates-full.md)（第 15 节）
