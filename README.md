# Quantix Connector Go

Go 后端 + HTML/JS 前端的工业设备连接器，提供称重、打印、扫码、看板和串口调试能力，支持 Windows/macOS 部署。

## 1. 功能概览

- 设备管理：创建设备、编辑设备、启停、删除、连接测试
- 实时看板：WebSocket 实时事件 + 轮询兜底
- 手动控制：执行 `manual` 步骤（如去皮/清零）
- 设备调试：
  - 打印：`/api/printers/{id}/print`
  - 扫码：`/api/scanners/{id}/last`
  - 看板：`/api/boards/{id}/status`
- 串口调试：端口扫描、开关连接、发送、读取、日志
- 协议模板：创建/编辑/删除/单步测试

## 2. 技术栈

- HTTP：Gin
- WebSocket：gorilla/websocket
- 数据库：GORM（SQLite/MySQL）
- 驱动：Modbus/MQTT/Serial/TCP

## 3. 快速启动

### 3.1 环境要求

- Go 1.22+
- （可选）Node.js，仅用于前端脚本静态检查
- Windows 或 macOS

### 3.2 启动服务

```bash
go run ./cmd/server
```

默认地址：

- 后端：`http://127.0.0.1:8000`
- 前端：`http://127.0.0.1:8000`（由后端静态托管）

## 4. 配置项（环境变量）

- `API_KEY`：默认 `quantix-dev-key`
- `BACKEND_HOST`：默认 `127.0.0.1`
- `BACKEND_PORT`：默认 `8000`
- `DB_TYPE`：默认 `sqlite`
- `DB_NAME`：默认 `quantix.db`
- `SIMULATE_ON_CONNECT_FAIL`：默认 `false`

## 5. 核心接口

- 健康检查：`GET /health`
- WebSocket：`GET /ws?api_key=...`
- 设备：
  - `GET/POST /api/devices`
  - `GET/PUT/DELETE /api/devices/{id}`
  - `POST /api/devices/{id}/enable|disable|execute`
  - `POST /api/devices/test-connection`（新增加，设备编辑页“测试连接”使用）
- 协议模板：
  - `GET/POST /api/protocols`
  - `GET/PUT/DELETE /api/protocols/{id}`
  - `POST /api/protocols/{id}/test-step`
- 串口调试：
  - `/api/serial-debug/ports|status|open|close|send|read|logs`

鉴权方式：

- Header：`X-API-Key`
- WebSocket：query `api_key`

## 6. 常见设备模板

请看文档：

- [docs/plan/common-device-templates.md](/Users/n/Code/Quantix-Cnnector-go/docs/plan/common-device-templates.md)

包括：

- `Std-Modbus-Scale`
- `MQTT-Weight-Sensor`
- `TSC-Serial-Print`
- `TSC-TCP-Print`
- `Serial-Scanner-LineMode`
- `Serial-Board-Polling`

## 7. Modbus TCP 本地联调

你可以使用旧仓库脚本：

- `/Users/n/Code/Quantix-Cnnector/tools/modbus_tcp_test_server.py`

示例（默认监听 `127.0.0.1:1502`）：

```bash
python3 /Users/n/Code/Quantix-Cnnector/tools/modbus_tcp_test_server.py --port 1502
```

设备连接参数建议：

```json
{
  "host": "127.0.0.1",
  "port": 1502
}
```

## 8. 开发与校验

```bash
go test ./...
node --check web/static/js/app.js
```

## 9. 稳定性建议（7x24）

- 生产环境使用固定串口命名与设备保活策略
- 对称重设备设置毫秒级采集间隔前，先评估 CPU/网络/设备响应上限
- 打印/扫码/看板分别做压力与异常恢复测试
- 所有上线前变更先在测试设备演练回退
