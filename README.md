# Quantix Connector Go

Go 后端 + HTML/JS 前端的工业设备连接器，提供称重、打印、扫码、看板和串口调试能力，支持 Windows/macOS 部署。


```bash
go run ./cmd/server
```


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

- 后端：默认监听 `0.0.0.0:8000`，允许局域网访问
- 本机访问：`http://127.0.0.1:8000`
- 局域网访问：`http://<本机局域网IP>:8000`

## 4. 配置项（环境变量）

- `API_KEY`：默认 `quantix-dev-key`
- `BACKEND_HOST`：默认 `0.0.0.0`
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
- 云打印代理：
  - `GET /api/print-agent/status`
  - `GET/PUT /api/print-agent/config`
  - `GET /api/print-agent/jobs`
  - `POST /api/print-agent/poll-once`

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

## 6.1 云打印代理（BarTender）

连接器已内置 Quantix 云打印代理，适用于：

- Quantix 服务端创建打印任务
- 本地连接器轮询任务
- 远程系统直连本地连接器提交打印任务
- 本机调用 BarTender 2022 打印 `.btw` 模板

推荐做法：

- `.btw` 模板放在客户端本机
- 在连接器 Web 页面“云打印”页签里配置：
  - Quantix 服务地址
  - `PRINT_AGENT_API_KEY`
  - 客户端 ID
  - 默认打印机名称
  - BarTender 可执行文件路径
  - `template_code -> 本地 .btw 路径` 映射

例如模板映射：

```json
{
  "material_label_v1": "D:/Code/Quantix/文档1.btw"
}
```

这样 Quantix 服务端只需要下发：

- `template_code=material_label_v1`
- `named_data`

推荐 `named_data` 字段（与 BarTender 命名数据源保持一致）：

```json
{
  "品名": "阿莫西林",
  "规格": "25kg/袋",
  "批号": "20260407001",
  "净重": "25.00kg",
  "皮重": "0.20kg",
  "毛重": "25.20kg",
  "存放人": "张三",
  "存放时间": "2026-04-14 09:30:00",
  "储存条件": "阴凉干燥处",
  "barcode": "MAT00120260407001"
}
```

连接器会在本地把任务映射到对应 `.btw` 模板并执行打印。

云打印支持双通道：

- 拉取模式：本地连接器主动请求远程服务端 `/api/print-jobs/next` 拉取任务。
- 直连模式：远程系统调用本地连接器 `POST /api/remote-print/jobs` 立即打印。

直连打印示例：

```http
POST /api/remote-print/jobs
X-API-Key: <connector_api_key>
Content-Type: application/json
```

```json
{
  "job_code": "JOB-20260512-001",
  "job_type": "bartender",
  "template_code": "material_label_v1",
  "printer_name": "",
  "copies": 1,
  "payload": {
    "品名": "阿莫西林",
    "批号": "20260407001",
    "barcode": "MAT00120260407001"
  }
}
```

直连模式复用云打印页签里的 `template_code -> 本地 .btw 路径` 映射、默认打印机、BarTender 路径和 `max_concurrent_jobs` 并发限制。生产环境开放直连接口时建议放在局域网、VPN 或内网穿透后面，并配置强 API Key。

远程直连打印对接文档：

- [docs/remote-direct-print.md](docs/remote-direct-print.md)

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

## 10. Windows 编译说明（无命令行窗口 + 高清图标）

### 前置要求

- 已安装 Go
- 已安装 `go-winres`
- 图标文件存在：`build/windows/quantix.ico`
  - 建议该 ico 内包含多尺寸（至少含 `256x256`）以保证高清显示

### 推荐编译命令（PowerShell）

在项目根目录执行：

```powershell
New-Item -ItemType Directory -Force bin | Out-Null
go-winres simply --arch amd64 --manifest gui --icon build/windows/quantix.ico
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -trimpath -ldflags "-s -w -H=windowsgui" -o bin/quantix-server.exe cmd/server/main.go
```

注意：当前 `go-winres simply` 不支持 `--in` 参数，不要写 `--in winres/winres.json`。如果命令输出 `flag provided but not defined: -in`，说明资源生成失败。

### 判断是否编译成功

```powershell
Test-Path bin/quantix-server.exe
Get-Item bin/quantix-server.exe | Select-Object FullName,Length,LastWriteTime
```

`Test-Path` 返回 `True`，并且 `LastWriteTime` 是本次编译时间，才表示 exe 已生成成功。

### Makefile 快捷命令

```powershell
make build-windows-gui
```

注意：Windows PowerShell 默认没有 `make`。如果没有安装 Make，直接使用上面的 PowerShell 编译命令即可。

### 编译参数说明

- `-trimpath`：移除文件系统路径，减小二进制大小
- `-ldflags "-s -w"`：去除调试信息和符号表，减小文件大小
- `-H=windowsgui`：隐藏命令行窗口，作为后台服务运行
- `go-winres`：嵌入 Windows 资源（图标、版本信息等）
