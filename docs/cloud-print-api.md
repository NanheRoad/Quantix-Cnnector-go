# 云打印接口文档

本文档描述 Quantix Connector 的云打印能力，分为三部分：

1. 本地 Connector 对外提供的云打印管理接口（`/api/print-agent/*`）
2. 本地 Connector 对外提供的远程直连打印接口（`/api/remote-print/*`）
3. Connector 作为打印代理，向云端 Quantix 服务调用的任务接口（`/api/print-jobs/*`）

云打印支持双通道同时存在：

- 拉取模式：Connector 主动请求云端 `/api/print-jobs/next`，适合内网、门店、车间和离线恢复。
- 直连模式：远程系统主动调用 Connector `/api/remote-print/jobs`，适合同局域网、VPN、专线或内网穿透。

两种模式复用同一套本地配置：`template_mappings`、默认打印机、BarTender 可执行文件路径、`max_concurrent_jobs` 并发限制和最近任务记录。

远程直连打印的独立对接指南见：[remote-direct-print.md](remote-direct-print.md)。

## 1. 认证与通用约定

- 本地 Connector 的受保护接口统一要求 API Key。
- 认证方式二选一：
  - 请求头：`X-API-Key: <your_api_key>`
  - Query：`?api_key=<your_api_key>`
- 内容类型：`application/json`
- 时间字段均为 RFC3339 格式（Go `time.Time` JSON 序列化）。

## 2. 本地云打印管理接口（Connector 提供）

Base URL 示例：`http://127.0.0.1:8000`

### 2.1 查询代理状态

- 方法与路径：`GET /api/print-agent/status`
- 说明：获取运行状态、最近轮询/成功/错误时间、统计计数等。

响应示例：

```json
{
  "enabled": true,
  "running": true,
  "worker_count": 1,
  "active_jobs": 0,
  "server_url": "http://quantix-server:8050",
  "client_id": "factory-tsc-01",
  "job_type": "bartender",
  "last_poll_at": "2026-04-14T08:12:10.123456Z",
  "last_success_at": "2026-04-14T08:12:09.998877Z",
  "last_error_at": "0001-01-01T00:00:00Z",
  "last_error": "",
  "current_job_code": "",
  "claimed_count": 120,
  "success_count": 118,
  "failed_count": 2
}
```

### 2.2 查询代理配置

- 方法与路径：`GET /api/print-agent/config`
- 说明：获取当前生效配置（内存中的配置快照）。

响应字段：

```json
{
  "enabled": true,
  "server_url": "http://quantix-server:8050",
  "agent_api_key": "*****",
  "client_id": "factory-tsc-01",
  "job_type": "bartender",
  "default_printer_name": "ZDesigner ZT410",
  "bartender_executable": "C:\\Program Files\\Seagull\\BarTender 2022\\BarTend.exe",
  "poll_interval_ms": 2000,
  "long_poll_ms": 15000,
  "max_concurrent_jobs": 1,
  "template_mappings": {
    "label_shipping": "D:\\labels\\shipping.btw",
    "label_box": "D:\\labels\\box.btw"
  }
}
```

### 2.3 查询 BarTender 可执行文件候选

- 方法与路径：`GET /api/print-agent/bartender-candidates`
- 说明：返回本机存在的默认安装路径候选列表。

响应示例：

```json
{
  "items": [
    "C:\\Program Files\\Seagull\\BarTender 2022\\BarTend.exe"
  ]
}
```

### 2.4 更新代理配置

- 方法与路径：`PUT /api/print-agent/config`
- 说明：保存配置到 `quantix.local.json`，并热更新运行中的打印代理。

请求体：

```json
{
  "enabled": true,
  "server_url": "http://quantix-server:8050",
  "agent_api_key": "quantix-print-agent-key",
  "client_id": "factory-tsc-01",
  "job_type": "bartender",
  "default_printer_name": "ZDesigner ZT410",
  "bartender_executable": "C:\\Program Files\\Seagull\\BarTender 2022\\BarTend.exe",
  "poll_interval_ms": 2000,
  "long_poll_ms": 15000,
  "max_concurrent_jobs": 1,
  "template_mappings": {
    "label_shipping": "D:\\labels\\shipping.btw"
  }
}
```

字段说明：

- `enabled`: 是否启用代理。
- `server_url`: 云端服务地址，保存时会去掉末尾 `/`。
- `agent_api_key`: Connector 调用云端任务接口时使用，放在请求头 `X-Print-Agent-Key`。
- `client_id`: 代理实例标识，用于拉取/回报任务。
- `job_type`: 任务类型；为空会归一化为 `bartender`。
- `default_printer_name`: 任务未指定打印机时的默认值。
- `bartender_executable`: BarTender 可执行文件路径；为空时将尝试系统默认候选路径。
- `poll_interval_ms`: 轮询间隔；`<=0` 时默认 `2000`。运行时最低生效间隔为 `500ms`。
- `long_poll_ms`: 拉取任务时传给云端的长轮询等待时长（毫秒）；`<=0` 默认 `15000`。
- `max_concurrent_jobs`: 并发 worker 数；`<=0` 默认 `1`，最大 `8`。
- `template_mappings`: 模板编码到 `.btw` 文件绝对/相对路径映射；空 key/value 会被清理。

### 2.5 查询最近任务记录

- 方法与路径：`GET /api/print-agent/jobs`
- 说明：返回最近任务历史，当前接口固定最多返回 50 条。

响应示例：

```json
{
  "items": [
    {
      "time": "2026-04-14T08:12:09.998877Z",
      "job_id": 901,
      "job_code": "JOB-20260414-000901",
      "template_code": "label_shipping",
      "printer_name": "ZDesigner ZT410",
      "status": "success",
      "message": "print completed",
      "result": {
        "template_path": "D:\\labels\\shipping.btw",
        "printer_name": "ZDesigner ZT410",
        "copies": 1,
        "bartender_exe": "C:\\Program Files\\Seagull\\BarTender 2022\\BarTend.exe",
        "output": ""
      }
    }
  ]
}
```

### 2.6 手动触发一次轮询

- 方法与路径：`POST /api/print-agent/poll-once`
- 说明：立即拉取并尝试执行一条云端任务（若有）。

成功响应：

```json
{ "ok": true }
```

### 2.7 远程直连提交打印任务

- 方法与路径：`POST /api/remote-print/jobs`
- 说明：远程系统直接调用本地 Connector 执行一次 BarTender 打印，不经过云端任务队列。
- 认证：使用本地 Connector API Key，即 `X-API-Key` 或 `?api_key=`。
- 执行方式：同步执行。接口会等待本地 BarTender 执行结束后返回成功或失败结果。
- 并发控制：直连打印和拉取打印共用 `max_concurrent_jobs` 限制，避免两条通道同时过量启动 BarTender。

请求体：

```json
{
  "job_code": "JOB-20260512-001",
  "job_type": "bartender",
  "template_code": "label_shipping",
  "printer_name": "ZDesigner ZT410",
  "copies": 1,
  "payload": {
    "sku": "ABC-001",
    "lot": "L20260512",
    "qty": "10"
  }
}
```

字段说明：

- `job_code`: 可选。为空时 Connector 会生成 `DIRECT-...` 编码。
- `job_type`: 可选，当前支持 `bartender`，为空时使用代理配置的 `job_type`。
- `template_code`: 必填。必须能在本地 `template_mappings` 中找到对应 `.btw` 文件。
- `printer_name`: 可选。为空时使用 `default_printer_name`。
- `copies`: 可选。`<=0` 时按 `1` 处理。
- `payload`: 可选。每个键值会映射为 BTXML 的 `NamedSubString`。

打印成功响应：

```json
{
  "ok": true,
  "status": "success",
  "message": "print completed",
  "job": {
    "time": "2026-05-12T10:00:00Z",
    "job_id": 0,
    "job_code": "JOB-20260512-001",
    "template_code": "label_shipping",
    "printer_name": "ZDesigner ZT410",
    "status": "success",
    "message": "print completed",
    "result": {
      "template_path": "D:\\labels\\shipping.btw",
      "printer_name": "ZDesigner ZT410",
      "copies": 1,
      "bartender_exe": "C:\\Program Files\\Seagull\\BarTender 2022\\BarTend.exe",
      "output": ""
    }
  }
}
```

打印失败响应仍为 `200 OK`，但 `ok=false`，方便调用方拿到本地执行结果：

```json
{
  "ok": false,
  "status": "failed",
  "message": "template mapping not found: label_shipping",
  "job": {
    "job_code": "JOB-20260512-001",
    "template_code": "label_shipping",
    "status": "failed",
    "message": "template mapping not found: label_shipping",
    "result": {
      "template_code": "label_shipping"
    }
  }
}
```

参数错误，例如缺少 `template_code`，返回 `400 Bad Request`。

## 3. 代理对云端任务接口契约（Connector 调用）

以下是 Connector 内置调用逻辑要求的云端接口契约。云端服务需实现这些接口，供打印代理对接。

通用要求：

- 请求头必须支持：`X-Print-Agent-Key: <agent_api_key>`
- `Content-Type: application/json`
- 2xx 表示成功；非 2xx 会被代理判定为失败并记录错误。

### 3.1 拉取下一条任务

- 方法与路径：`POST /api/print-jobs/next`

请求体：

```json
{
  "client_id": "factory-tsc-01",
  "job_type": "bartender"
}
```

成功响应体（必须包含 `success`）：

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 901,
    "job_code": "JOB-20260414-000901",
    "template_code": "label_shipping",
    "printer_name": "ZDesigner ZT410",
    "copies": 1,
    "payload": {
      "sku": "ABC-001",
      "lot": "L20260414"
    }
  }
}
```

无任务建议返回：

```json
{
  "success": true,
  "message": "",
  "data": null
}
```

若 `success=false`，代理将把 `message` 作为错误处理。

### 3.2 回报任务成功

- 方法与路径：`POST /api/print-jobs/{id}/success`

请求体：

```json
{
  "client_id": "factory-tsc-01",
  "result": {
    "template_path": "D:\\labels\\shipping.btw",
    "printer_name": "ZDesigner ZT410",
    "copies": 1,
    "output": ""
  }
}
```

响应体：任意 JSON（2xx 即视为成功）。

### 3.3 回报任务失败

- 方法与路径：`POST /api/print-jobs/{id}/failed`

请求体：

```json
{
  "client_id": "factory-tsc-01",
  "error_message": "template mapping not found: label_shipping",
  "result": {
    "template_code": "label_shipping"
  }
}
```

响应体：任意 JSON（2xx 即视为成功）。

## 4. 本地执行逻辑说明（BarTender）

1. 根据 `template_code` 在 `template_mappings` 查找 `.btw` 路径；未命中即失败。
2. 校验模板文件存在；不存在即失败。
3. 解析 BarTender 可执行文件路径：
   - 优先使用配置项 `bartender_executable`
   - 否则尝试默认安装路径候选
4. 生成临时 BTXML 并执行：
   - 命令：`BarTend.exe /XMLScript=<temp.btxml> /X`
5. 执行成功后回报 `/success`；失败回报 `/failed`。

字段补充：

- `copies <= 0` 时自动按 `1` 处理。
- 任务未给 `printer_name` 时使用 `default_printer_name`。
- `payload` 的每个键值会映射为 BTXML 的 `NamedSubString`。

## 5. 错误码与错误响应

本地接口常见状态码：

- `200 OK`: 请求成功。
- `400 Bad Request`: 参数错误、代理禁用、业务校验不通过。
- `401 Unauthorized`: API Key 无效。
- `503 Service Unavailable`: 打印代理实例不可用。
- `500 Internal Server Error`: 配置持久化等服务端错误。

错误响应统一示例：

```json
{ "detail": "error message" }
```

## 6. 相关接口（本地设备打印）

除云打印代理外，Connector 还提供直接触发打印步骤接口：

- `POST /api/printers/{device_id}/print`

用途：对 `device_category=printer_tsc` 的本地设备执行手动打印步骤。  
请求体：

```json
{
  "step_id": "print_send",
  "params": {
    "print_data": "TEXT 10,10,\"FONT001\",0,1,1,\"Hello\""
  }
}
```

`step_id` 可省略；省略时系统会在协议模板中自动查找第一个可打印手动步骤。
