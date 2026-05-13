# 远程直连打印对接文档

更新时间：2026-05-13

本文档说明远程系统如何直接调用本地 Quantix Connector 执行 BarTender 打印。

## 1. 工作模式

远程直连打印是“远程系统主动请求本机 Connector”的模式：

```text
远程系统 / Quantix 服务端
        |
        | POST /api/remote-print/jobs
        v
本地 Quantix Connector
        |
        | 调用 BarTender
        v
本地打印机
```

它可以和云端拉取模式同时启用：

- 云端拉取：本机 Connector 主动去远程服务器拉任务，适合内网、离线恢复、无公网入口。
- 远程直连：远程系统直接请求本机 Connector，适合局域网、VPN、专线或内网穿透，延迟更低。

两种模式复用同一套云打印配置，包括模板映射、默认打印机、BarTender 路径和并发限制。

## 2. 使用前提

本机 Connector 需要完成以下配置：

1. 在 Web 页面进入“云打印”页签。
2. 配置 BarTender 可执行文件。
3. 配置默认打印机，或让请求里传 `printer_name`。
4. 配置模板映射，例如：

```json
{
  "material_label_v1": "D:/labels/material_label_v1.btw"
}
```

远程系统还必须能访问本机 Connector 地址。例如：

- 同一局域网：`http://192.168.1.50:8000`
- VPN / 专线：使用 VPN 内网地址
- 内网穿透：使用穿透后的 HTTPS 地址

## 3. 接口信息

### 3.1 提交直连打印任务

```http
POST /api/remote-print/jobs
X-API-Key: <connector_api_key>
Content-Type: application/json
```

完整 URL 示例：

```text
http://192.168.1.50:8000/api/remote-print/jobs
```

认证使用本地 Connector 的 API Key，不是云端打印代理的 `agent_api_key`。

## 4. 请求体

```json
{
  "job_code": "JOB-20260513-001",
  "job_type": "bartender",
  "template_code": "material_label_v1",
  "printer_name": "",
  "copies": 1,
  "payload": {
    "品名": "阿莫西林",
    "规格": "25kg/袋",
    "批号": "20260513001",
    "净重": "25.00kg",
    "barcode": "MAT00120260513001"
  }
}
```

字段说明：

| 字段 | 必填 | 说明 |
|---|---:|---|
| `job_code` | 否 | 业务任务编号。为空时 Connector 自动生成 `DIRECT-...` |
| `job_type` | 否 | 当前支持 `bartender`。为空时使用云打印配置里的任务类型 |
| `template_code` | 是 | 模板编码，必须存在于本地 `template_mappings` |
| `printer_name` | 否 | 打印机名称。为空时使用默认打印机 |
| `copies` | 否 | 打印份数，`<=0` 时按 `1` 处理 |
| `payload` | 否 | 打印数据，键名会映射到 BarTender NamedSubString |

## 5. cURL 示例

```bash
curl -X POST "http://192.168.1.50:8000/api/remote-print/jobs" \
  -H "X-API-Key: quantix-dev-key" \
  -H "Content-Type: application/json" \
  -d '{
    "job_code": "JOB-20260513-001",
    "job_type": "bartender",
    "template_code": "material_label_v1",
    "printer_name": "",
    "copies": 1,
    "payload": {
      "品名": "阿莫西林",
      "批号": "20260513001",
      "barcode": "MAT00120260513001"
    }
  }'
```

## 6. 响应

### 6.1 打印成功

```json
{
  "ok": true,
  "status": "success",
  "message": "print completed",
  "job": {
    "time": "2026-05-13T10:00:00Z",
    "job_id": 0,
    "job_code": "JOB-20260513-001",
    "template_code": "material_label_v1",
    "printer_name": "",
    "status": "success",
    "message": "print completed",
    "result": {
      "template_path": "D:\\labels\\material_label_v1.btw",
      "printer_name": "",
      "copies": 1,
      "bartender_exe": "C:\\Program Files\\Seagull\\BarTender 2022\\BarTend.exe",
      "output": ""
    }
  }
}
```

### 6.2 打印失败

打印执行失败时通常仍返回 HTTP `200`，但业务字段 `ok=false`：

```json
{
  "ok": false,
  "status": "failed",
  "message": "template mapping not found: material_label_v1",
  "job": {
    "job_code": "JOB-20260513-001",
    "template_code": "material_label_v1",
    "status": "failed",
    "message": "template mapping not found: material_label_v1",
    "result": {
      "template_code": "material_label_v1"
    }
  }
}
```

参数错误，例如缺少 `template_code`，返回 HTTP `400`：

```json
{
  "detail": "template_code is required"
}
```

API Key 错误返回 HTTP `401`：

```json
{
  "detail": "Invalid API key"
}
```

## 7. 并发与队列

远程直连打印和云端拉取打印共用 `max_concurrent_jobs` 限制。

默认建议：

```json
{
  "max_concurrent_jobs": 1
}
```

多数 BarTender + 单打印机场景建议保持 `1`，避免多个打印任务同时抢同一台打印机。

## 8. 安全建议

远程直连会让外部系统直接触发本机打印，生产环境建议：

- 不要裸露到公网。
- 优先使用局域网、VPN、专线或可信内网穿透。
- 配置强 API Key。
- 限制来源 IP。
- 使用 HTTPS。
- 不允许远程传本地模板路径，只允许传 `template_code`。
- 控制 `copies` 最大值，避免误打大量标签。
- 对 `job_code` 做幂等处理，避免业务系统重复提交导致重复打印。

## 9. 常见问题

### 9.1 远程系统访问不到本机地址

先在远程系统所在机器上访问：

```text
http://<connector-host>:8000/health
```

如果打不开，优先检查：

- Connector 是否正在运行
- Windows 防火墙是否放行端口
- 远程机器和本机是否在同一网络/VPN
- 路由、端口映射或内网穿透配置是否正确

### 9.2 返回 `template mapping not found`

说明 `template_code` 没有在本机云打印配置的 `template_mappings` 中配置。

例如请求传：

```json
{
  "template_code": "material_label_v1"
}
```

本机配置必须有：

```json
{
  "material_label_v1": "D:/labels/material_label_v1.btw"
}
```

### 9.3 BarTender 找不到或执行失败

检查云打印配置里的 BarTender 可执行文件路径，或在 Web 页面点击候选路径重新选择。

### 9.4 打印数据没有进入标签

确认 `payload` 的键名和 BarTender 模板中的 NamedSubString 名称一致。例如：

```json
{
  "品名": "阿莫西林",
  "批号": "20260513001",
  "barcode": "MAT00120260513001"
}
```

BarTender 模板中也需要存在同名数据源：`品名`、`批号`、`barcode`。
